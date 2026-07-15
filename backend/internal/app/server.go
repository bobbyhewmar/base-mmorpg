package app

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/coder/websocket"
)

type Server struct {
	addr          string
	publicWSURL   string
	mux           *http.ServeMux
	attachedMu    sync.Mutex
	partyMu       sync.Mutex
	clanMu        sync.Mutex
	store         *Store
	config        ServerConfig
	corsOrigins   map[string]struct{}
	authLimiter   *fixedWindowRateLimiter
	attachLimiter *fixedWindowRateLimiter
	attached      map[string]*attachedSession
	pendingTrades map[string]*playerTradeOffer
	observer      *observer
}

type attachedSession struct {
	sessionID   string
	runtime     *attachedRuntime
	send        func(map[string]any) bool
	ready       bool
	dispatchMu  sync.Mutex
	movementMu  sync.Mutex
	pendingMove *pendingMovementDispatch
}

type pendingMovementDispatch struct {
	requestToken uint64
	commandID    string
	commandSeq   int
	cancel       context.CancelFunc
}

func (attached *attachedSession) dispatchAll(build func(*attachedRuntime) []map[string]any) bool {
	if attached == nil || attached.runtime == nil || attached.send == nil || build == nil {
		return false
	}

	attached.dispatchMu.Lock()
	defer attached.dispatchMu.Unlock()

	for _, payload := range build(attached.runtime) {
		if payload == nil {
			continue
		}
		if !attached.send(payload) {
			return false
		}
	}
	return true
}

func (attached *attachedSession) sendSerialized(payload map[string]any) bool {
	if payload == nil {
		return true
	}
	return attached.dispatchAll(func(*attachedRuntime) []map[string]any {
		return []map[string]any{payload}
	})
}

func (attached *attachedSession) replacePendingMove(next *pendingMovementDispatch) *pendingMovementDispatch {
	if attached == nil {
		return nil
	}

	attached.movementMu.Lock()
	defer attached.movementMu.Unlock()

	previous := attached.pendingMove
	attached.pendingMove = next
	return previous
}

func (attached *attachedSession) clearPendingMove(requestToken uint64) *pendingMovementDispatch {
	if attached == nil {
		return nil
	}

	attached.movementMu.Lock()
	defer attached.movementMu.Unlock()

	if attached.pendingMove == nil {
		return nil
	}
	if requestToken != 0 && attached.pendingMove.requestToken != requestToken {
		return nil
	}
	previous := attached.pendingMove
	attached.pendingMove = nil
	return previous
}

func NewServer(addr string, publicWSURL string, store *Store) *Server {
	return NewServerWithConfig(addr, publicWSURL, store, ServerConfig{})
}

func NewServerWithConfig(addr string, publicWSURL string, store *Store, config ServerConfig) *Server {
	if store == nil {
		store = newMemoryStore()
	}
	config = normalizeServerConfig(config)
	corsOrigins := make(map[string]struct{}, len(config.AllowedOrigins))
	for _, origin := range config.AllowedOrigins {
		corsOrigins[origin] = struct{}{}
	}
	s := &Server{
		addr:          addr,
		publicWSURL:   publicWSURL,
		mux:           http.NewServeMux(),
		store:         store,
		config:        config,
		corsOrigins:   corsOrigins,
		authLimiter:   newFixedWindowRateLimiter(config.AuthRateLimit),
		attachLimiter: newFixedWindowRateLimiter(config.AttachRateLimit),
		attached:      map[string]*attachedSession{},
		pendingTrades: map[string]*playerTradeOffer{},
		observer:      newObserver(),
	}
	s.routes()
	return s
}

func (s *Server) Start() error {
	log.Printf("backend stub listening on %s", s.addr)
	s.observer.log("info", "server_start", map[string]any{
		"addr":       s.addr,
		"store_mode": s.store.Mode,
	})
	return http.ListenAndServe(s.addr, s.handler())
}

func (s *Server) handler() http.Handler {
	return s.withObservability(s.withCORS(s.mux))
}

func (s *Server) resolveGameplayWSURL(r *http.Request) string {
	if s.publicWSURL != "" {
		return s.publicWSURL
	}

	scheme := "ws"
	if r.TLS != nil {
		scheme = "wss"
	}
	return fmt.Sprintf("%s://%s/v1/gameplay/ws", scheme, r.Host)
}

func originPatternsForHost(originScheme string, host string) []string {
	patternSet := map[string]struct{}{
		host:                        {},
		originScheme + "://" + host: {},
	}

	hostName, port, err := net.SplitHostPort(host)
	if err != nil {
		hostName = host
		port = ""
	}
	if hostName == "localhost" || hostName == "127.0.0.1" {
		alternateHost := "127.0.0.1"
		if hostName == "127.0.0.1" {
			alternateHost = "localhost"
		}
		alternate := alternateHost
		if port != "" {
			alternate = net.JoinHostPort(alternateHost, port)
		}
		patternSet[alternate] = struct{}{}
		patternSet[originScheme+"://"+alternate] = struct{}{}
	}

	patterns := make([]string, 0, len(patternSet))
	for pattern := range patternSet {
		patterns = append(patterns, pattern)
	}
	sort.Strings(patterns)
	return patterns
}

func (s *Server) gameplayWSOriginPatterns(r *http.Request) ([]string, error) {
	if s.publicWSURL != "" {
		parsedURL, err := url.Parse(s.publicWSURL)
		if err != nil {
			return nil, fmt.Errorf("invalid public websocket URL: %w", err)
		}
		if parsedURL.Host == "" {
			return nil, errors.New("invalid public websocket URL: missing host")
		}

		switch parsedURL.Scheme {
		case "ws":
			return originPatternsForHost("http", parsedURL.Host), nil
		case "wss":
			return originPatternsForHost("https", parsedURL.Host), nil
		default:
			return nil, fmt.Errorf("invalid public websocket URL scheme: %s", parsedURL.Scheme)
		}
	}

	if r == nil || r.Host == "" {
		return nil, errors.New("missing request host for direct websocket origin validation")
	}

	originScheme := "http"
	if r.TLS != nil {
		originScheme = "https"
	}
	return originPatternsForHost(originScheme, r.Host), nil
}

func (s *Server) routes() {
	s.mux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	})
	s.mux.HandleFunc("/metrics", s.handleMetrics)
	s.mux.HandleFunc("/v1/auth/register", s.handleRegister)
	s.mux.HandleFunc("/v1/auth/login", s.handleLogin)
	s.mux.HandleFunc("/v1/characters", s.handleCharacters)
	s.mux.HandleFunc("/v1/characters/catalog", s.handleCharactersCatalog)
	s.mux.HandleFunc("/v1/world/enter", s.handleWorldEnter)
	s.mux.HandleFunc("/v1/gameplay/ws", s.handleGameplayWS)
	s.mux.HandleFunc("/internal/economy/events", s.handleInternalEconomyEvents)
	s.mux.HandleFunc("/internal/economy/warehouse-transfers", s.handleInternalWarehouseTransfers)
	s.mux.HandleFunc("/internal/economy/trades", s.handleInternalTradeEvents)
}

func (s *Server) handleMetrics(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "protocol.method_not_allowed", "Method not allowed.")
		return
	}
	w.Header().Set("Content-Type", "text/plain; version=0.0.4; charset=utf-8")
	_, _ = w.Write([]byte(s.observer.renderPrometheus()))
}

func (s *Server) withCORS(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.EqualFold(r.Header.Get("Upgrade"), "websocket") {
			next.ServeHTTP(w, r)
			return
		}

		origin := strings.TrimSpace(r.Header.Get("Origin"))
		if origin != "" {
			if len(s.corsOrigins) == 0 {
				http.Error(w, "cross-origin requests are not enabled", http.StatusForbidden)
				return
			}
			if _, allowed := s.corsOrigins[origin]; !allowed {
				http.Error(w, "origin not allowed", http.StatusForbidden)
				return
			}
			w.Header().Set("Vary", "Origin")
			w.Header().Set("Access-Control-Allow-Origin", origin)
			w.Header().Set("Access-Control-Allow-Headers", "Authorization, Content-Type")
			w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
		}
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (s *Server) withObservability(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.EqualFold(r.Header.Get("Upgrade"), "websocket") {
			next.ServeHTTP(w, r)
			return
		}

		startedAt := time.Now()
		recorder := newResponseRecorder(w)
		next.ServeHTTP(recorder, r)

		statusCode := recorder.statusCode
		statusText := strconv.Itoa(statusCode)
		labels := map[string]string{
			"method":      r.Method,
			"path":        r.URL.Path,
			"status_code": statusText,
		}
		duration := time.Since(startedAt)
		s.observer.incCounter("l2bg_http_requests_total", "Total HTTP requests handled by the backend.", labels, 1)
		s.observer.observeDurationSeconds("l2bg_http_request_duration_seconds", "HTTP request duration in seconds.", labels, duration)
		if statusCode >= http.StatusBadRequest {
			s.observer.incCounter("l2bg_http_errors_total", "Total HTTP requests that completed with an error status.", labels, 1)
		}
		s.observer.log("info", "http_request", map[string]any{
			"method":       r.Method,
			"path":         r.URL.Path,
			"status_code":  statusCode,
			"duration_ms":  duration.Milliseconds(),
			"remote_ip":    requestRemoteIP(r),
			"content_type": strings.TrimSpace(r.Header.Get("Content-Type")),
		})
	})
}

type apiError struct {
	ReasonCode string `json:"reason_code"`
	Message    string `json:"message"`
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}

func writeError(w http.ResponseWriter, status int, reasonCode, message string) {
	writeJSON(w, status, apiError{
		ReasonCode: reasonCode,
		Message:    message,
	})
}

func decodeJSON(r *http.Request, target any) error {
	defer r.Body.Close()
	return json.NewDecoder(r.Body).Decode(target)
}

func randomID(prefix string) string {
	var bytes [8]byte
	if _, err := rand.Read(bytes[:]); err != nil {
		return fmt.Sprintf("%s_%d", prefix, time.Now().UnixNano())
	}
	return fmt.Sprintf("%s_%s", prefix, hex.EncodeToString(bytes[:]))
}

func normalizeName(name string) string {
	return strings.ToLower(strings.TrimSpace(name))
}

type raceDefinition struct {
	Race             string
	BaseClasses      []string
	SexOptions       []string
	HairStyleOptions []int
	SkinTypeOptions  []int
	DefaultHairColor string
}

const defaultHairColor = "#6b4e37"

func canonicalHairStyleOptions() []int {
	return []int{0, 1, 2}
}

func canonicalSkinTypeOptions() []int {
	return []int{0, 1, 2}
}

func canonicalBaseClasses() []string {
	return []string{"Fighter", "Mage"}
}

func validRaces() map[string]raceDefinition {
	baseClasses := canonicalBaseClasses()
	return map[string]raceDefinition{
		"Human": {
			Race:             "Human",
			BaseClasses:      baseClasses,
			SexOptions:       []string{"Male", "Female"},
			HairStyleOptions: canonicalHairStyleOptions(),
			SkinTypeOptions:  canonicalSkinTypeOptions(),
			DefaultHairColor: defaultHairColor,
		},
	}
}

func orderedRaces() []raceDefinition {
	races := validRaces()
	keys := make([]string, 0, len(races))
	for key := range races {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	definitions := make([]raceDefinition, 0, len(keys))
	for _, key := range keys {
		definitions = append(definitions, races[key])
	}
	return definitions
}

func validSexes() map[string]struct{} {
	return map[string]struct{}{
		"Male":   {},
		"Female": {},
	}
}

func intOptionAllowed(options []int, value int) bool {
	for _, option := range options {
		if option == value {
			return true
		}
	}
	return false
}

func normalizeCanonicalHairColor(value string) (string, bool) {
	normalized := strings.ToLower(strings.TrimSpace(value))
	if len(normalized) != 7 || normalized[0] != '#' {
		return "", false
	}
	for _, character := range normalized[1:] {
		if !(character >= '0' && character <= '9' || character >= 'a' && character <= 'f') {
			return "", false
		}
	}
	return normalized, true
}

func (s *Server) handleRegister(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "protocol.method_not_allowed", "Method not allowed.")
		return
	}

	var request struct {
		Login       string `json:"login"`
		Password    string `json:"password"`
		DisplayName string `json:"display_name"`
	}
	if err := decodeJSON(r, &request); err != nil {
		writeError(w, http.StatusBadRequest, "protocol.invalid_envelope", "Invalid register request.")
		return
	}

	login := strings.TrimSpace(strings.ToLower(request.Login))
	if !s.allowAuthAttempt(r, login) {
		writeError(w, http.StatusTooManyRequests, "auth.rate_limited", "Too many authentication attempts. Please try again later.")
		return
	}
	if login == "" {
		writeError(w, http.StatusBadRequest, "auth.login_unavailable", "Login is required.")
		return
	}
	if len(request.Password) < 8 {
		writeError(w, http.StatusBadRequest, "auth.password_policy_failed", "Password policy failed.")
		return
	}

	account := &Account{
		ID:          randomID("acc"),
		Login:       login,
		DisplayName: strings.TrimSpace(request.DisplayName),
		State:       accountStateActive,
	}
	if strings.Contains(login, "+pending") {
		account.State = accountStatePendingVerification
	}

	credential, err := newCredentialRecord(account.ID, request.Password)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "system.persistence_failed", "Unable to hash password.")
		return
	}
	if err := s.store.CreateAccountWithCredential(r.Context(), account, credential); err != nil {
		if errors.Is(err, errRecordConflict) {
			writeError(w, http.StatusConflict, "auth.login_unavailable", "Login is unavailable.")
			return
		}
		s.recordStoreError("accounts.create_with_credential", err, errRecordConflict)
		writeError(w, http.StatusInternalServerError, "system.persistence_failed", "Unable to persist account.")
		return
	}

	registrationState := "created_active"
	nextStep := "login"
	if account.State == accountStatePendingVerification {
		registrationState = "created_pending_verification"
		nextStep = "login_or_verify"
	}

	writeJSON(w, http.StatusCreated, map[string]any{
		"account_id":         account.ID,
		"registration_state": registrationState,
		"next_step":          nextStep,
	})
}

func (s *Server) handleLogin(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "protocol.method_not_allowed", "Method not allowed.")
		return
	}

	var request struct {
		Login    string `json:"login"`
		Password string `json:"password"`
	}
	if err := decodeJSON(r, &request); err != nil {
		writeError(w, http.StatusBadRequest, "protocol.invalid_envelope", "Invalid login request.")
		return
	}

	login := strings.TrimSpace(strings.ToLower(request.Login))
	if !s.allowAuthAttempt(r, login) {
		writeError(w, http.StatusTooManyRequests, "auth.rate_limited", "Too many authentication attempts. Please try again later.")
		return
	}

	account, credential, err := s.store.GetByLoginWithCredential(r.Context(), login)
	if err != nil {
		if errors.Is(err, errRecordNotFound) {
			writeError(w, http.StatusUnauthorized, "auth.invalid_credentials", "Invalid login or password.")
			return
		}
		s.recordStoreError("accounts.get_by_login_with_credential", err, errRecordNotFound)
		writeError(w, http.StatusInternalServerError, "system.persistence_failed", "Unable to load account.")
		return
	}
	matched, needsUpgrade, err := verifyPassword(request.Password, credential)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "system.persistence_failed", "Unable to verify credentials.")
		return
	}
	if !matched {
		writeError(w, http.StatusUnauthorized, "auth.invalid_credentials", "Invalid login or password.")
		return
	}
	if needsUpgrade {
		upgradedCredential, err := newCredentialRecord(account.ID, request.Password)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "system.persistence_failed", "Unable to upgrade password security.")
			return
		}
		if err := s.store.UpdateCredential(r.Context(), upgradedCredential); err != nil {
			s.recordStoreError("credentials.update", err)
			writeError(w, http.StatusInternalServerError, "system.persistence_failed", "Unable to upgrade password security.")
			return
		}
	}

	switch account.State {
	case accountStatePendingVerification:
		writeError(w, http.StatusForbidden, "auth.account_unverified", "Account verification is still pending.")
		return
	case accountStateLocked:
		writeError(w, http.StatusLocked, "auth.account_locked", "Account is locked.")
		return
	}

	now := time.Now()
	accountSession := newAccountSession(account.ID, s.config.AccessTokenTTL, now)
	if err := s.store.AccountSessions.Create(r.Context(), accountSession); err != nil {
		s.recordStoreError("account_sessions.create", err)
		writeError(w, http.StatusInternalServerError, "system.persistence_failed", "Unable to persist account session.")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"account_id":    account.ID,
		"access_token":  accountSession.Token,
		"expires_at_ms": accountSession.ExpiresAt.UnixMilli(),
		"account_state": account.State,
	})
}

func (s *Server) handleCharacters(w http.ResponseWriter, r *http.Request) {
	account, err := s.requireAccount(r)
	if err != nil {
		writeError(w, http.StatusUnauthorized, "auth.not_authenticated", "Missing or invalid access token.")
		return
	}

	switch r.Method {
	case http.MethodGet:
		characters, err := s.store.Characters.ListByAccountID(r.Context(), account.ID)
		if err != nil {
			s.recordStoreError("characters.list_by_account", err)
			writeError(w, http.StatusInternalServerError, "system.persistence_failed", "Unable to load characters.")
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"characters": characters})
	case http.MethodPost:
		s.handleCreateCharacter(w, r, account)
	default:
		writeError(w, http.StatusMethodNotAllowed, "protocol.method_not_allowed", "Method not allowed.")
	}
}

func (s *Server) handleCharactersCatalog(w http.ResponseWriter, r *http.Request) {
	if _, err := s.requireAccount(r); err != nil {
		writeError(w, http.StatusUnauthorized, "auth.not_authenticated", "Missing or invalid access token.")
		return
	}
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "protocol.method_not_allowed", "Method not allowed.")
		return
	}

	races := make([]map[string]any, 0, len(validRaces()))
	for _, definition := range orderedRaces() {
		races = append(races, map[string]any{
			"race":         definition.Race,
			"enabled":      true,
			"base_classes": definition.BaseClasses,
			"sex_options":  definition.SexOptions,
			"appearance_options": map[string]any{
				"hair_styles":        definition.HairStyleOptions,
				"hair_color_default": definition.DefaultHairColor,
				"skin_types":         definition.SkinTypeOptions,
			},
		})
	}
	writeJSON(w, http.StatusOK, map[string]any{"races": races})
}

func (s *Server) handleCreateCharacter(w http.ResponseWriter, r *http.Request, account *Account) {
	var request struct {
		Race      string `json:"race"`
		BaseClass string `json:"base_class"`
		Sex       string `json:"sex"`
		HairStyle *int   `json:"hair_style"`
		HairColor string `json:"hair_color"`
		SkinType  *int   `json:"skin_type"`
		Name      string `json:"name"`
	}
	if err := decodeJSON(r, &request); err != nil {
		writeError(w, http.StatusBadRequest, "protocol.invalid_envelope", "Invalid character creation request.")
		return
	}

	count, err := s.store.Characters.CountByAccountID(r.Context(), account.ID)
	if err != nil {
		s.recordStoreError("characters.count_by_account", err)
		writeError(w, http.StatusInternalServerError, "system.persistence_failed", "Unable to load character count.")
		return
	}
	if count >= 5 {
		writeError(w, http.StatusConflict, "character.creation_limit_reached", "Character creation limit reached.")
		return
	}

	raceDefinition, validRace := validRaces()[request.Race]
	if !validRace {
		writeError(w, http.StatusBadRequest, "character.invalid_race", "Race is invalid.")
		return
	}

	validBaseClass := false
	for _, baseClass := range raceDefinition.BaseClasses {
		if baseClass == request.BaseClass {
			validBaseClass = true
			break
		}
	}
	if !validBaseClass {
		writeError(w, http.StatusBadRequest, "character.base_class_not_allowed_for_race", "Base class is invalid for the selected race.")
		return
	}

	if _, ok := validSexes()[request.Sex]; !ok {
		writeError(w, http.StatusBadRequest, "character.invalid_sex", "Sex is invalid.")
		return
	}
	validSex := false
	for _, sexOption := range raceDefinition.SexOptions {
		if sexOption == request.Sex {
			validSex = true
			break
		}
	}
	if !validSex {
		writeError(w, http.StatusBadRequest, "character.sex_not_allowed_for_template", "Sex is not allowed for the selected template.")
		return
	}

	if request.HairStyle == nil || !intOptionAllowed(raceDefinition.HairStyleOptions, *request.HairStyle) {
		writeError(w, http.StatusBadRequest, "character.invalid_hair_style", "Hairstyle is invalid for the selected template.")
		return
	}
	hairColor, validHairColor := normalizeCanonicalHairColor(request.HairColor)
	if !validHairColor {
		writeError(w, http.StatusBadRequest, "character.invalid_hair_color", "Hair color must be a canonical #RRGGBB value.")
		return
	}
	if request.SkinType == nil || !intOptionAllowed(raceDefinition.SkinTypeOptions, *request.SkinType) {
		writeError(w, http.StatusBadRequest, "character.invalid_skin_type", "Skin type is invalid for the selected template.")
		return
	}

	if normalizeName(request.Name) == "" {
		writeError(w, http.StatusBadRequest, "character.invalid_name", "Character name is invalid.")
		return
	}
	if len([]rune(strings.TrimSpace(request.Name))) < 3 {
		writeError(w, http.StatusBadRequest, "character.name_too_short", "Character name is too short.")
		return
	}
	if len([]rune(strings.TrimSpace(request.Name))) > 24 {
		writeError(w, http.StatusBadRequest, "character.name_too_long", "Character name is too long.")
		return
	}
	for _, r := range request.Name {
		if !(r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == ' ') {
			writeError(w, http.StatusBadRequest, "character.name_contains_invalid_characters", "Character name contains invalid characters.")
			return
		}
	}

	character := &Character{
		ID:           randomID("char"),
		Name:         strings.TrimSpace(request.Name),
		Race:         request.Race,
		BaseClass:    request.BaseClass,
		Sex:          request.Sex,
		HairStyle:    *request.HairStyle,
		HairColor:    hairColor,
		SkinType:     *request.SkinType,
		Level:        1,
		LastRegionID: startingRegionID,
		PositionX:    startingPositionX,
		PositionZ:    startingPositionZ,
		IsEnterable:  true,
		AccountID:    account.ID,
	}
	initialItems := initialCharacterItemSeed(character)
	if err := s.store.CreateCharacterWithItemSeed(r.Context(), character, initialItems); err != nil {
		if errors.Is(err, errRecordConflict) {
			writeError(w, http.StatusConflict, "character.name_unavailable", "Character name is unavailable.")
			return
		}
		s.recordStoreError("characters.create_with_item_seed", err, errRecordConflict)
		writeError(w, http.StatusInternalServerError, "system.persistence_failed", "Unable to persist character.")
		return
	}

	characters, err := s.store.Characters.ListByAccountID(r.Context(), account.ID)
	if err != nil {
		s.recordStoreError("characters.list_by_account", err)
		writeError(w, http.StatusInternalServerError, "system.persistence_failed", "Unable to load characters.")
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{
		"character":  character,
		"characters": characters,
	})
}

func (s *Server) handleWorldEnter(w http.ResponseWriter, r *http.Request) {
	account, err := s.requireAccount(r)
	if err != nil {
		writeError(w, http.StatusUnauthorized, "auth.not_authenticated", "Missing or invalid access token.")
		return
	}
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "protocol.method_not_allowed", "Method not allowed.")
		return
	}

	var request struct {
		CharacterID string `json:"character_id"`
	}
	if err := decodeJSON(r, &request); err != nil {
		writeError(w, http.StatusBadRequest, "protocol.invalid_envelope", "Invalid world-enter request.")
		return
	}

	character, err := s.store.Characters.GetByID(r.Context(), request.CharacterID)
	if err != nil {
		if errors.Is(err, errRecordNotFound) {
			writeError(w, http.StatusForbidden, "character.not_owned", "Character is not owned by this account.")
			return
		}
		s.recordStoreError("characters.get_by_id", err, errRecordNotFound)
		writeError(w, http.StatusInternalServerError, "system.persistence_failed", "Unable to load character.")
		return
	}
	if character.AccountID != account.ID {
		writeError(w, http.StatusForbidden, "character.not_owned", "Character is not owned by this account.")
		return
	}
	items, err := s.store.Items.ListByCharacterID(r.Context(), character.ID)
	if err != nil {
		s.recordStoreError("items.list_by_character", err)
		writeError(w, http.StatusInternalServerError, "system.persistence_failed", "Unable to load character inventory.")
		return
	}
	now := time.Now()
	cooldowns, err := s.store.CharacterCooldowns.ListByCharacterID(r.Context(), character.ID)
	if err != nil {
		s.recordStoreError("character_cooldowns.list_by_character", err)
		writeError(w, http.StatusInternalServerError, "system.persistence_failed", "Unable to load character cooldown state.")
		return
	}
	hotbarState, err := s.loadCharacterHotbarState(r.Context(), character)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "system.persistence_failed", "Unable to load character hotbar state.")
		return
	}
	pets, err := s.loadCharacterPets(r.Context(), character.ID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "system.persistence_failed", "Unable to load character pet state.")
		return
	}
	questState, err := s.loadCharacterQuestState(r.Context(), character.ID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "system.persistence_failed", "Unable to load character quest state.")
		return
	}
	partyState, partyInvites, err := s.loadCharacterPartyState(r.Context(), character.ID, now)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "system.persistence_failed", "Unable to load character party state.")
		return
	}
	clanState, clanInvites, err := s.loadCharacterClanState(r.Context(), character.ID, now)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "system.persistence_failed", "Unable to load character clan state.")
		return
	}
	cooldownSnapshot := cooldownSnapshotFromRecords(cooldowns, now)
	selfState := selfStateFromItems(character, items, cooldownSnapshot, hotbarState, pets, questState, partyState, partyInvites, clanState, clanInvites)

	if err := s.store.GameplaySessions.ExpireStalePendingAttach(r.Context(), character.ID, now); err != nil {
		s.recordStoreError("gameplay_sessions.expire_stale_pending_attach", err)
		writeError(w, http.StatusInternalServerError, "system.persistence_failed", "Unable to expire stale gameplay sessions.")
		return
	}

	pendingSession, err := s.store.GameplaySessions.GetLatestPendingForCharacter(r.Context(), character.ID, now)
	if err != nil {
		if !errors.Is(err, errRecordNotFound) {
			s.recordStoreError("gameplay_sessions.get_latest_pending_for_character", err, errRecordNotFound)
			writeError(w, http.StatusInternalServerError, "system.persistence_failed", "Unable to inspect gameplay sessions.")
			return
		}
		pendingSession = nil
	}
	if pendingSession != nil {
		wsURL := s.resolveGameplayWSURL(r)
		writeJSON(w, http.StatusOK, map[string]any{
			"session_id":           pendingSession.ID,
			"character_id":         character.ID,
			"attach_token":         pendingSession.AttachToken,
			"attach_expires_at_ms": pendingSession.AttachExpiresAt.UnixMilli(),
			"self_state":           selfState,
			"item_state":           snapshotCharacterItems(items),
			"ws_url":               wsURL,
		})
		return
	}

	hasAttachedSession, err := s.store.GameplaySessions.HasAttachedForCharacter(r.Context(), character.ID)
	if err != nil {
		s.recordStoreError("gameplay_sessions.has_attached_for_character", err)
		writeError(w, http.StatusInternalServerError, "system.persistence_failed", "Unable to inspect gameplay sessions.")
		return
	}
	if hasAttachedSession {
		writeError(w, http.StatusConflict, "session.character_already_active", "Character already has an active or pending session.")
		return
	}

	session := &Session{
		ID:              randomID("sess"),
		AccountID:       account.ID,
		CharacterID:     character.ID,
		AttachToken:     randomID("attach"),
		AttachExpiresAt: time.Now().Add(5 * time.Minute),
		Status:          sessionStatusPendingAttach,
	}
	if err := s.store.GameplaySessions.Create(r.Context(), session); err != nil {
		if errors.Is(err, errRecordConflict) {
			writeError(w, http.StatusConflict, "session.character_already_active", "Character already has an active or pending session.")
			return
		}
		s.recordStoreError("gameplay_sessions.create", err, errRecordConflict)
		writeError(w, http.StatusInternalServerError, "system.persistence_failed", "Unable to persist gameplay session.")
		return
	}

	wsURL := s.resolveGameplayWSURL(r)
	writeJSON(w, http.StatusOK, map[string]any{
		"session_id":           session.ID,
		"character_id":         character.ID,
		"attach_token":         session.AttachToken,
		"attach_expires_at_ms": session.AttachExpiresAt.UnixMilli(),
		"self_state":           selfState,
		"item_state":           snapshotCharacterItems(items),
		"ws_url":               wsURL,
	})
}

func (s *Server) handleGameplayWS(w http.ResponseWriter, r *http.Request) {
	if !s.allowAttachAttempt(r) {
		s.recordAttachAttempt("rate_limited", "auth.rate_limited", "", "")
		http.Error(w, "gameplay websocket rate limited", http.StatusTooManyRequests)
		return
	}

	originPatterns, err := s.gameplayWSOriginPatterns(r)
	if err != nil {
		log.Printf("gameplay websocket origin policy error: %v", err)
		s.recordAttachAttempt("server_error", "system.origin_policy_misconfigured", "", "")
		http.Error(w, "gameplay websocket origin policy misconfigured", http.StatusServiceUnavailable)
		return
	}

	conn, err := websocket.Accept(w, r, &websocket.AcceptOptions{
		OriginPatterns: originPatterns,
	})
	if err != nil {
		return
	}
	defer conn.CloseNow()
	s.observer.addGauge("l2bg_websocket_connections_active", "Currently active websocket connections.", nil, 1)
	defer s.observer.addGauge("l2bg_websocket_connections_active", "Currently active websocket connections.", nil, -1)

	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	defer cancel()

	_, payload, err := conn.Read(ctx)
	if err != nil {
		s.recordAttachAttempt("rejected", "session.attach_message_missing", "", "")
		return
	}

	var attach struct {
		Kind        string `json:"kind"`
		SessionID   string `json:"session_id"`
		AttachToken string `json:"attach_token"`
	}
	if err := json.Unmarshal(payload, &attach); err != nil || attach.Kind != "attach_session" {
		s.writeWSReject(ctx, conn, "", 0, "session.not_attachable", "First websocket message must be attach_session.")
		s.recordAttachAttempt("rejected", "session.not_attachable", "", "")
		_ = conn.Close(websocket.StatusPolicyViolation, "invalid attach")
		return
	}

	session, character, err := s.attachSession(attach.SessionID, attach.AttachToken)
	if err != nil {
		s.writeWSReject(ctx, conn, "", 0, err.Error(), wsReasonMessage(err))
		s.recordAttachAttempt("rejected", err.Error(), attach.SessionID, "")
		_ = conn.Close(websocket.StatusPolicyViolation, "attach rejected")
		return
	}

	runtime, err := s.buildAttachedRuntime(r.Context(), session, character, time.Now())
	if err != nil {
		s.writeWSReject(ctx, conn, "", 0, "system.persistence_failed", "Unable to load character runtime state.")
		s.recordAttachAttempt("rejected", "system.persistence_failed", session.ID, character.ID)
		_ = conn.Close(websocket.StatusInternalError, "attach runtime load failed")
		return
	}
	s.recordAttachAttempt("accepted", "none", session.ID, character.ID)
	defer func() {
		s.persistCharacterWorldState(session.CharacterID, runtime)
		s.persistCharacterProgression(session.CharacterID, runtime)
		s.persistCharacterCooldownState(session.CharacterID, runtime)
		s.persistCharacterQuestState(session.CharacterID, runtime)
		s.closeAttachedSession(session.ID)
	}()
	loopCtx, cancelLoop := context.WithCancel(r.Context())
	defer cancelLoop()
	outboundCh := make(chan map[string]any, 32)
	writeErrCh := make(chan error, 1)

	go func() {
		for {
			select {
			case <-loopCtx.Done():
				return
			case payload := <-outboundCh:
				if err := writeWSJSON(loopCtx, conn, payload); err != nil {
					select {
					case writeErrCh <- err:
					default:
					}
					cancelLoop()
					return
				}
				s.recordOutboundMessage(payload)
			}
		}
	}()

	sendOutbound := func(payload map[string]any) bool {
		select {
		case outboundCh <- payload:
			return true
		case <-loopCtx.Done():
			return false
		}
	}
	s.stageAttachedSession(session.ID, runtime, sendOutbound)
	defer s.unregisterAttachedSession(session.ID)
	attached := s.attachedSessionBySessionID(session.ID)
	if !sendOutbound(runtime.regionContextMessage()) {
		return
	}
	for _, outbound := range s.activateAttachedSession(session.ID) {
		if !sendOutbound(outbound) {
			return
		}
	}
	s.fanOutPartyStateForCharacterExcept(loopCtx, session.CharacterID, session.CharacterID)

	go func() {
		ticker := time.NewTicker(250 * time.Millisecond)
		defer ticker.Stop()
		for {
			select {
			case <-loopCtx.Done():
				return
			case now := <-ticker.C:
				var movementChanged bool
				var respawned bool
				var tickMessages []map[string]any
				if attached == nil || !attached.dispatchAll(func(runtime *attachedRuntime) []map[string]any {
					tickMessages, movementChanged, respawned = runtime.collectTickMessagesWithStore(now, s.store)
					return tickMessages
				}) {
					return
				}
				s.fanOutWorldEntityVisibility(session.ID, runtime, tickMessages)
				if movementChanged {
					s.fanOutPresenceState(session.ID, runtime)
				}
				if containsResolvedCommandDelta(tickMessages) {
					s.persistCharacterProgression(session.CharacterID, runtime)
					s.persistCharacterCooldownState(session.CharacterID, runtime)
					s.persistCharacterQuestState(session.CharacterID, runtime)
				}
				if respawned {
					s.persistCharacterProgression(session.CharacterID, runtime)
					if !movementChanged {
						s.fanOutPresenceState(session.ID, runtime)
					}
				}
				if attached != nil && attached.runtime != nil {
					if attached.runtime.partyInviteExpirationDue(now) {
						s.sendPartyStateRefresh(loopCtx, session.CharacterID)
					}
					if attached.runtime.clanInviteExpirationDue(now) {
						s.sendClanStateRefresh(loopCtx, session.CharacterID)
					}
				}
			}
		}
	}()

	for {
		select {
		case <-loopCtx.Done():
			return
		case err := <-writeErrCh:
			if err != nil {
				return
			}
		default:
		}

		_, payload, err := conn.Read(loopCtx)
		if err != nil {
			return
		}
		var command commandEnvelope
		if err := json.Unmarshal(payload, &command); err != nil {
			if !sendOutbound(rejectMessage("", 0, "protocol.invalid_envelope", "Gameplay command envelope is invalid.")) {
				return
			}
			continue
		}
		var outboundMessages []map[string]any
		var shouldFanOut bool
		if attached == nil || !attached.dispatchAll(func(runtime *attachedRuntime) []map[string]any {
			if command.Type == "move_intent" {
				outboundMessages = s.processAsyncMovementCommandWithDedup(loopCtx, session, attached, runtime, command)
				shouldFanOut = false
			} else {
				outboundMessages, shouldFanOut = s.processGameplayCommandWithDedup(loopCtx, session, runtime, command)
			}
			return outboundMessages
		}) {
			return
		}
		if shouldFanOut {
			s.fanOutWorldEntityVisibility(session.ID, runtime, outboundMessages)
			if command.Type == "move_intent" || command.Type == "use_skill" || command.Type == "basic_attack" || command.Type == "use_item" || command.Type == "tame_mob" || command.Type == "summon_pet" || command.Type == "dismiss_pet" || command.Type == "mount_pet" || command.Type == "dismount_pet" {
				s.fanOutPresenceState(session.ID, runtime)
			}
		}
	}
}

func (s *Server) writeWSReject(ctx context.Context, conn *websocket.Conn, commandID string, commandSeq int, reasonCode, message string) {
	payload := map[string]any{
		"kind":          "reject",
		"emitted_at_ms": time.Now().UnixMilli(),
		"reason_code":   reasonCode,
		"message":       message,
	}
	if commandID != "" {
		payload["command_id"] = commandID
		payload["command_seq"] = commandSeq
	}
	if err := writeWSJSON(ctx, conn, payload); err == nil {
		s.recordOutboundMessage(payload)
	}
}

func writeWSJSON(ctx context.Context, conn *websocket.Conn, payload any) error {
	bytes, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	return conn.Write(ctx, websocket.MessageText, bytes)
}

func (s *Server) registerAttachedSession(sessionID string, runtime *attachedRuntime, send func(map[string]any) bool) {
	if runtime == nil || send == nil {
		return
	}
	s.attachedMu.Lock()
	s.attached[sessionID] = &attachedSession{
		sessionID: sessionID,
		runtime:   runtime,
		send:      send,
		ready:     true,
	}
	s.attachedMu.Unlock()
	s.observer.addGauge("l2bg_attached_sessions_active", "Currently attached gameplay sessions.", nil, 1)
	s.observer.addGauge("l2bg_region_occupancy", "Currently attached gameplay sessions by region.", map[string]string{
		"region_id": metricRegionID(runtime.regionIDValue()),
	}, 1)
}

func (s *Server) stageAttachedSession(sessionID string, runtime *attachedRuntime, send func(map[string]any) bool) {
	if runtime == nil || send == nil {
		return
	}

	s.attachedMu.Lock()
	for peerSessionID, attached := range s.attached {
		if peerSessionID == sessionID || attached == nil || attached.runtime == nil || !attached.ready {
			continue
		}
		if attached.runtime.regionIDValue() != runtime.regionIDValue() {
			continue
		}
		runtime.seedKnownEntity(attached.runtime.playerPresenceEntity())
		if petEntity, exists := attached.runtime.activePetEntity(); exists && petEntity != nil {
			runtime.seedKnownEntity(*petEntity)
		}
	}
	s.attached[sessionID] = &attachedSession{
		sessionID: sessionID,
		runtime:   runtime,
		send:      send,
		ready:     false,
	}
	s.attachedMu.Unlock()
	s.observer.addGauge("l2bg_attached_sessions_active", "Currently attached gameplay sessions.", nil, 1)
	s.observer.addGauge("l2bg_region_occupancy", "Currently attached gameplay sessions by region.", map[string]string{
		"region_id": metricRegionID(runtime.regionIDValue()),
	}, 1)
}

func (s *Server) activateAttachedSession(sessionID string) []map[string]any {
	s.attachedMu.Lock()
	attached := s.attached[sessionID]
	if attached == nil || attached.runtime == nil {
		s.attachedMu.Unlock()
		return nil
	}
	attached.ready = true
	regionID := attached.runtime.regionIDValue()
	peers := make([]*attachedSession, 0, len(s.attached))
	for peerSessionID, peer := range s.attached {
		if peerSessionID == sessionID || peer == nil || peer.runtime == nil || peer.send == nil || !peer.ready {
			continue
		}
		if peer.runtime.regionIDValue() != regionID {
			continue
		}
		peers = append(peers, peer)
	}
	s.attachedMu.Unlock()

	messagesToSelf := make([]map[string]any, 0, len(peers))
	for _, peer := range peers {
		if message := attached.runtime.applyRemotePlayerAppear(peer.runtime.playerPresenceEntity()); message != nil {
			messagesToSelf = append(messagesToSelf, message)
		}
		if petEntity, exists := peer.runtime.activePetEntity(); exists && petEntity != nil {
			if message := attached.runtime.applyRemoteEntityAppear(*petEntity); message != nil {
				messagesToSelf = append(messagesToSelf, message)
			}
		}
	}

	selfEntity := attached.runtime.playerPresenceEntity()
	selfPetEntity, hasSelfPetEntity := attached.runtime.activePetEntity()
	for _, peer := range peers {
		_ = peer.dispatchAll(func(runtime *attachedRuntime) []map[string]any {
			messages := make([]map[string]any, 0, 2)
			if message := runtime.applyRemotePlayerAppear(selfEntity); message != nil {
				messages = append(messages, message)
			}
			if hasSelfPetEntity && selfPetEntity != nil {
				if message := runtime.applyRemoteEntityAppear(*selfPetEntity); message != nil {
					messages = append(messages, message)
				}
			}
			return messages
		})
	}

	return messagesToSelf
}

func (s *Server) readyRegionTargets(sourceSessionID string, regionID string) []*attachedSession {
	if regionID == "" {
		return nil
	}

	s.attachedMu.Lock()
	targets := make([]*attachedSession, 0, len(s.attached))
	for sessionID, attached := range s.attached {
		if sessionID == sourceSessionID || attached == nil || attached.runtime == nil || attached.send == nil || !attached.ready {
			continue
		}
		if attached.runtime.regionIDValue() != regionID {
			continue
		}
		targets = append(targets, attached)
	}
	s.attachedMu.Unlock()
	return targets
}

func (s *Server) attachedSessionBySessionID(sessionID string) *attachedSession {
	if s == nil || sessionID == "" {
		return nil
	}

	s.attachedMu.Lock()
	defer s.attachedMu.Unlock()

	return s.attached[sessionID]
}

func (s *Server) unregisterAttachedSession(sessionID string) {
	s.attachedMu.Lock()
	attached := s.attached[sessionID]
	delete(s.attached, sessionID)
	s.attachedMu.Unlock()
	if pending := attached.clearPendingMove(0); pending != nil {
		pending.cancel()
		s.finalizeSupersededMovementOutcome(sessionID, pending)
	}
	if attached == nil || attached.runtime == nil {
		return
	}
	for _, notification := range s.clearPendingTradesForSession(sessionID) {
		notification.send(notification.payload)
	}
	if attached.ready {
		for _, target := range s.readyRegionTargets(sessionID, attached.runtime.regionIDValue()) {
			_ = target.dispatchAll(func(runtime *attachedRuntime) []map[string]any {
				messages := make([]map[string]any, 0, 2)
				if message := runtime.applyRemotePlayerDisappear(attached.runtime.characterID, entityDisappearPlayer); message != nil {
					messages = append(messages, message)
				}
				if petMessages := runtime.syncRemotePetPresence(attached.runtime.characterID, nil); len(petMessages) > 0 {
					messages = append(messages, petMessages...)
				}
				return messages
			})
		}
	}
	s.fanOutPartyStateForCharacter(context.Background(), attached.runtime.characterID)
	s.observer.addGauge("l2bg_attached_sessions_active", "Currently attached gameplay sessions.", nil, -1)
	s.observer.addGauge("l2bg_region_occupancy", "Currently attached gameplay sessions by region.", map[string]string{
		"region_id": metricRegionID(attached.runtime.regionIDValue()),
	}, -1)
}

func (s *Server) fanOutWorldEntityVisibility(sourceSessionID string, sourceRuntime *attachedRuntime, outboundMessages []map[string]any) {
	if sourceRuntime == nil || len(outboundMessages) == 0 {
		return
	}

	sourceRegionID := sourceRuntime.regionIDValue()
	if sourceRegionID == "" {
		return
	}

	targets := s.readyRegionTargets(sourceSessionID, sourceRegionID)
	if len(targets) == 0 {
		return
	}

	for _, outbound := range outboundMessages {
		kind, _ := outbound["kind"].(string)
		switch kind {
		case "entity_appear":
			entity, ok := outbound["entity"].(runtimeEntity)
			if !ok || entity.EntityType == "player" {
				continue
			}
			for _, target := range targets {
				_ = target.dispatchAll(func(runtime *attachedRuntime) []map[string]any {
					if message := runtime.applyRemoteEntityAppear(entity); message != nil {
						return []map[string]any{message}
					}
					return nil
				})
			}
		case "entity_disappear":
			entityID, _ := outbound["entity_id"].(string)
			reason, _ := outbound["reason"].(string)
			if entityID == "" {
				continue
			}
			for _, target := range targets {
				_ = target.dispatchAll(func(runtime *attachedRuntime) []map[string]any {
					if message := runtime.applyRemoteEntityDisappear(entityID, reason); message != nil {
						return []map[string]any{message}
					}
					return nil
				})
			}
		}
	}
}

func (s *Server) fanOutPresenceState(sourceSessionID string, sourceRuntime *attachedRuntime) {
	if sourceRuntime == nil {
		return
	}

	sourceRegionID := sourceRuntime.regionIDValue()
	if sourceRegionID == "" {
		return
	}
	targets := s.readyRegionTargets(sourceSessionID, sourceRegionID)
	if len(targets) == 0 {
		return
	}

	entity := sourceRuntime.playerPresenceEntity()
	petEntity, hasPetEntity := sourceRuntime.activePetEntity()
	for _, target := range targets {
		_ = target.dispatchAll(func(runtime *attachedRuntime) []map[string]any {
			messages := make([]map[string]any, 0, 2)
			if message := runtime.applyRemotePlayerState(entity); message != nil {
				messages = append(messages, message)
			}
			if hasPetEntity && petEntity != nil {
				messages = append(messages, runtime.syncRemotePetPresence(sourceRuntime.characterID, petEntity)...)
			} else {
				messages = append(messages, runtime.syncRemotePetPresence(sourceRuntime.characterID, nil)...)
			}
			return messages
		})
	}
}

func (s *Server) fanOutLootVisibility(sourceSessionID string, sourceRuntime *attachedRuntime, outboundMessages []map[string]any) {
	s.fanOutWorldEntityVisibility(sourceSessionID, sourceRuntime, outboundMessages)
}

func (s *Server) fanOutPlayerState(sourceSessionID string, sourceRuntime *attachedRuntime) {
	s.fanOutPresenceState(sourceSessionID, sourceRuntime)
}

func (s *Server) attachSession(sessionID, attachToken string) (*Session, *Character, error) {
	ctx := context.Background()

	session, err := s.store.GameplaySessions.GetByID(ctx, sessionID)
	if err != nil {
		if errors.Is(err, errRecordNotFound) {
			return nil, nil, errors.New("session.not_found")
		}
		s.recordStoreError("gameplay_sessions.get_by_id", err, errRecordNotFound)
		return nil, nil, errors.New("session.not_found")
	}
	if session.Status == sessionStatusAttached {
		return nil, nil, errors.New("session.already_attached")
	}
	if time.Now().After(session.AttachExpiresAt) {
		if err := s.store.GameplaySessions.UpdateStatus(ctx, session.ID, sessionStatusExpired); err != nil {
			s.recordStoreError("gameplay_sessions.update_status", err)
		}
		return nil, nil, errors.New("session.expired")
	}
	if session.AttachToken != attachToken {
		return nil, nil, errors.New("session.invalid_attach_token")
	}
	character, err := s.store.Characters.GetByID(ctx, session.CharacterID)
	if err != nil {
		s.recordStoreError("characters.get_by_id", err)
		return nil, nil, errors.New("session.not_attachable")
	}
	if err := s.store.GameplaySessions.UpdateStatus(ctx, session.ID, sessionStatusAttached); err != nil {
		s.recordStoreError("gameplay_sessions.update_status", err)
		return nil, nil, errors.New("session.not_attachable")
	}
	session.Status = sessionStatusAttached
	return session, character, nil
}

func (s *Server) closeAttachedSession(sessionID string) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	characterID := ""
	s.attachedMu.Lock()
	if attached := s.attached[sessionID]; attached != nil && attached.runtime != nil {
		characterID = attached.runtime.characterID
	}
	s.attachedMu.Unlock()

	session, err := s.store.GameplaySessions.GetByID(ctx, sessionID)
	if err != nil {
		s.recordStoreError("gameplay_sessions.get_by_id", err, errRecordNotFound)
		if characterID != "" {
			s.expirePartyInvitesForDisconnectedCharacter(ctx, characterID)
			s.expireClanInvitesForDisconnectedCharacter(ctx, characterID)
		}
		return
	}
	if session.Status != sessionStatusAttached {
		if characterID == "" {
			characterID = session.CharacterID
		}
		if characterID != "" {
			s.expirePartyInvitesForDisconnectedCharacter(ctx, characterID)
			s.expireClanInvitesForDisconnectedCharacter(ctx, characterID)
		}
		return
	}
	if err := s.store.GameplaySessions.UpdateStatus(ctx, sessionID, sessionStatusClosed); err != nil {
		s.recordStoreError("gameplay_sessions.update_status", err)
	}
	if characterID == "" {
		characterID = session.CharacterID
	}
	s.expirePartyInvitesForDisconnectedCharacter(ctx, characterID)
	s.expireClanInvitesForDisconnectedCharacter(ctx, characterID)
}

func (s *Server) persistCharacterWorldState(characterID string, runtime *attachedRuntime) {
	if runtime == nil {
		return
	}

	regionID, position := runtime.characterWorldState()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	if err := s.store.Characters.UpdateWorldState(ctx, characterID, regionID, position.X, position.Z); err != nil {
		s.recordStoreError("characters.update_world_state", err)
	}
}

func (s *Server) persistCharacterProgression(characterID string, runtime *attachedRuntime) {
	if runtime == nil {
		return
	}

	level, xp, currentCP, currentHP, currentMP := runtime.characterProgressionState()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	if err := s.store.Characters.UpdateProgression(ctx, characterID, level, xp, currentCP, currentHP, currentMP); err != nil {
		s.recordStoreError("characters.update_progression", err)
	}
}

func (s *Server) persistCharacterCooldownState(characterID string, runtime *attachedRuntime) {
	if runtime == nil || s.store == nil || s.store.CharacterCooldowns == nil {
		return
	}

	cooldowns := runtime.characterCooldownState(time.Now())
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	if err := s.store.CharacterCooldowns.ReplaceByCharacterID(ctx, characterID, cooldowns); err != nil {
		s.recordStoreError("character_cooldowns.replace_by_character", err)
	}
}

func (s *Server) persistCharacterQuestState(characterID string, runtime *attachedRuntime) {
	if runtime == nil || s.store == nil || s.store.CharacterQuests == nil {
		return
	}

	quest := runtime.characterQuestState()
	quest.CharacterID = characterID
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	if err := s.store.CharacterQuests.UpsertByCharacterID(ctx, quest); err != nil {
		s.recordStoreError("character_quests.upsert_by_character", err)
	}
}

func (s *Server) buildAttachedRuntime(ctx context.Context, session *Session, character *Character, now time.Time) (*attachedRuntime, error) {
	if session == nil || character == nil {
		return nil, errors.New("missing runtime context")
	}

	items, err := s.store.Items.ListByCharacterID(ctx, character.ID)
	if err != nil {
		s.recordStoreError("items.list_by_character", err)
		return nil, err
	}
	cooldowns, err := s.store.CharacterCooldowns.ListByCharacterID(ctx, character.ID)
	if err != nil {
		s.recordStoreError("character_cooldowns.list_by_character", err)
		return nil, err
	}
	hotbarState, err := s.loadCharacterHotbarState(ctx, character)
	if err != nil {
		return nil, err
	}
	pets, err := s.loadCharacterPets(ctx, character.ID)
	if err != nil {
		return nil, err
	}
	questState, err := s.loadCharacterQuestState(ctx, character.ID)
	if err != nil {
		return nil, err
	}
	partyState, partyInvites, err := s.loadCharacterPartyState(ctx, character.ID, now)
	if err != nil {
		return nil, err
	}
	clanState, clanInvites, err := s.loadCharacterClanState(ctx, character.ID, now)
	if err != nil {
		return nil, err
	}

	runtime := newCleanAttachedRuntime(session.ID, character)
	runtime.deferRewardResolution = true
	runtime.characterItems = cloneCharacterItems(items)
	runtime.derivedStats = deriveCharacterStats(character, items)
	runtime.hotbarState = hotbarState
	runtime.loadPetState(pets)
	runtime.loadCooldownState(cooldowns, now)
	runtime.loadQuestState([]CharacterQuestState{questState})
	runtime.loadPartyState(partyState, partyInvites)
	runtime.loadClanState(clanState, clanInvites)
	runtime.reconcileResourcePools()
	return runtime, nil
}

func (s *Server) loadCharacterHotbarState(ctx context.Context, character *Character) (CharacterHotbarState, error) {
	if character == nil {
		return CharacterHotbarState{}, errors.New("missing character")
	}

	hotbarState, err := s.store.CharacterHotbars.ListByCharacterID(ctx, character.ID)
	if err == nil {
		return normalizeCharacterHotbarState(hotbarState, character), nil
	}
	if errors.Is(err, errRecordNotFound) {
		return defaultCharacterHotbarState(character), nil
	}

	s.recordStoreError("character_hotbars.list_by_character", err)
	return CharacterHotbarState{}, err
}

func (s *Server) loadCharacterQuestState(ctx context.Context, characterID string) (CharacterQuestState, error) {
	if characterID == "" {
		return CharacterQuestState{}, errors.New("missing character")
	}
	if s == nil || s.store == nil || s.store.CharacterQuests == nil {
		state := defaultCharacterQuestState()
		state.CharacterID = characterID
		return state, nil
	}

	quests, err := s.store.CharacterQuests.ListByCharacterID(ctx, characterID)
	if err == nil {
		return primaryQuestState(quests, characterID), nil
	}
	if errors.Is(err, errRecordNotFound) {
		state := defaultCharacterQuestState()
		state.CharacterID = characterID
		return state, nil
	}

	s.recordStoreError("character_quests.list_by_character", err)
	return CharacterQuestState{}, err
}

func (s *Server) loadCharacterPets(ctx context.Context, characterID string) ([]CharacterPet, error) {
	if characterID == "" {
		return nil, errors.New("missing character")
	}
	if s == nil || s.store == nil || s.store.CharacterPets == nil {
		return nil, nil
	}

	pets, err := s.store.CharacterPets.ListByCharacterID(ctx, characterID)
	if err == nil {
		return normalizeCharacterPets(pets), nil
	}
	if errors.Is(err, errRecordNotFound) {
		return nil, nil
	}

	s.recordStoreError("character_pets.list_by_character", err)
	return nil, err
}

func containsPlayerRespawnDelta(outboundMessages []map[string]any) bool {
	for _, outbound := range outboundMessages {
		kind, _ := outbound["kind"].(string)
		if kind != "delta" {
			continue
		}
		self, ok := outbound["self"].(map[string]any)
		if !ok {
			continue
		}
		dead, _ := self["dead"].(bool)
		if !dead {
			return true
		}
	}
	return false
}

func containsResolvedCommandDelta(outboundMessages []map[string]any) bool {
	for _, outbound := range outboundMessages {
		kind, _ := outbound["kind"].(string)
		if kind != "delta" {
			continue
		}
		commandID, _ := outbound["applies_to_command_id"].(string)
		if commandID != "" {
			return true
		}
	}
	return false
}

func metricRegionID(regionID string) string {
	regionID = strings.TrimSpace(regionID)
	if regionID == "" {
		return "unknown"
	}
	return regionID
}

func metricCommandType(commandType string) string {
	commandType = strings.TrimSpace(commandType)
	if commandType == "" {
		return "unknown"
	}
	return commandType
}

func extractRejectReason(outboundMessages []map[string]any) string {
	for _, outbound := range outboundMessages {
		kind, _ := outbound["kind"].(string)
		if kind != "reject" {
			continue
		}
		reasonCode, _ := outbound["reason_code"].(string)
		if reasonCode != "" {
			return reasonCode
		}
	}
	return ""
}

func commandOutcomeFromOutbound(outboundMessages []map[string]any) string {
	hasApply := false
	for _, outbound := range outboundMessages {
		kind, _ := outbound["kind"].(string)
		switch kind {
		case "delta", "entity_appear", "entity_disappear", "position_correction", tradeNoticeKind, partyNoticeKind, chatMessageKind:
			hasApply = true
		case "reject":
			return "rejected"
		}
	}
	if hasApply {
		return "applied"
	}
	return "acknowledged"
}

func (s *Server) recordAttachAttempt(result string, reasonCode string, sessionID string, characterID string) {
	if s == nil || s.observer == nil {
		return
	}

	if reasonCode == "" {
		reasonCode = "none"
	}
	s.observer.incCounter("l2bg_ws_attach_attempts_total", "Total websocket attach outcomes.", map[string]string{
		"result":      result,
		"reason_code": reasonCode,
	}, 1)
	s.observer.log("info", "websocket_attach", map[string]any{
		"result":       result,
		"reason_code":  reasonCode,
		"session_id":   sessionID,
		"character_id": characterID,
	})
}

func (s *Server) recordOutboundMessage(payload map[string]any) {
	if s == nil || s.observer == nil || payload == nil {
		return
	}

	kind, _ := payload["kind"].(string)
	if kind == "" {
		kind = "unknown"
	}
	s.observer.incCounter("l2bg_gameplay_outbound_messages_total", "Total outbound gameplay websocket messages by kind.", map[string]string{
		"kind": kind,
	}, 1)

	if kind != "reject" {
		return
	}

	reasonCode, _ := payload["reason_code"].(string)
	if reasonCode == "" {
		reasonCode = "unknown"
	}
	s.observer.incCounter("l2bg_gameplay_rejects_total", "Total gameplay rejects by reason code.", map[string]string{
		"reason_code": reasonCode,
	}, 1)
}

func (s *Server) recordCommandObservation(sessionID string, command commandEnvelope, outboundMessages []map[string]any, result string, duration time.Duration) {
	if s == nil || s.observer == nil {
		return
	}

	commandType := metricCommandType(command.Type)
	if result == "" {
		result = commandOutcomeFromOutbound(outboundMessages)
	}
	labels := map[string]string{
		"command_type": commandType,
		"result":       result,
	}
	s.observer.incCounter("l2bg_gameplay_commands_total", "Total gameplay commands processed by outcome.", labels, 1)
	s.observer.observeDurationSeconds("l2bg_gameplay_command_duration_seconds", "Gameplay command duration in seconds.", labels, duration)

	fields := map[string]any{
		"session_id":   sessionID,
		"command_id":   command.CommandID,
		"command_seq":  command.CommandSeq,
		"command_type": commandType,
		"result":       result,
		"duration_ms":  duration.Milliseconds(),
	}
	if reasonCode := extractRejectReason(outboundMessages); reasonCode != "" {
		fields["reason_code"] = reasonCode
	}
	s.observer.log("info", "gameplay_command", fields)
}

func (s *Server) recordStoreError(operation string, err error, ignored ...error) {
	if s == nil || s.observer == nil || err == nil {
		return
	}
	for _, ignoredErr := range ignored {
		if errors.Is(err, ignoredErr) {
			return
		}
	}

	storeMode := "unknown"
	if s.store != nil && s.store.Mode != "" {
		storeMode = s.store.Mode
	}
	s.observer.incCounter("l2bg_db_errors_total", "Total persistence errors on critical backend paths.", map[string]string{
		"operation":  operation,
		"store_mode": storeMode,
	}, 1)
	s.observer.log("error", "persistence_error", map[string]any{
		"operation":  operation,
		"store_mode": storeMode,
		"error":      err.Error(),
	})
}

func wsReasonMessage(err error) string {
	switch err.Error() {
	case "session.not_found":
		return "Session was not found."
	case "session.already_attached":
		return "Session is already attached."
	case "session.expired":
		return "Session attach token is expired."
	case "session.invalid_attach_token":
		return "Attach token is invalid."
	default:
		return "Session is not attachable."
	}
}

func (s *Server) allowAuthAttempt(r *http.Request, login string) bool {
	now := time.Now()
	for _, key := range authRateLimitKeys(r, login) {
		if !s.authLimiter.Allow(key, now) {
			return false
		}
	}
	return true
}

func (s *Server) allowAttachAttempt(r *http.Request) bool {
	return s.attachLimiter.Allow("ip:"+requestRemoteIP(r), time.Now())
}

func (s *Server) requireAccount(r *http.Request) (*Account, error) {
	token := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
	if token == "" {
		return nil, errors.New("missing token")
	}

	accountSession, err := s.store.AccountSessions.GetActiveByToken(r.Context(), token, time.Now())
	if err != nil {
		s.recordStoreError("account_sessions.get_active_by_token", err, errRecordNotFound)
		return nil, errors.New("unknown token")
	}
	account, err := s.store.Accounts.GetByID(r.Context(), accountSession.AccountID)
	if err != nil {
		s.recordStoreError("accounts.get_by_id", err, errRecordNotFound)
		return nil, errors.New("unknown account")
	}
	return account, nil
}
