import * as THREE from 'three';
import type { Object3D } from 'three';
import { gameTemplates } from '../data/templates';
import { getEquippedBySlot, getTargetMob, getTemplate, type GameStore } from '../domain/game';
import { selectClickTarget, type ClickTarget, type ClickHit } from './picking';
import type {
  AppearanceOptionIndex,
  BaseClass,
  CharacterRace,
  CharacterSex,
  GameState,
  Vec2,
} from '../domain/types';

type MobVisual = {
  group: THREE.Group;
  hpBar: THREE.Mesh;
};

type CompanionVisual = MobVisual;

type PlayerVisual = {
  group: THREE.Group;
  proceduralRoot: THREE.Group;
  torso: THREE.Mesh;
  hip: THREE.Mesh;
  head: THREE.Mesh;
  hair: THREE.Mesh;
  faceMark: THREE.Mesh;
  armLeft: THREE.Mesh;
  armRight: THREE.Mesh;
  legLeft: THREE.Mesh;
  legRight: THREE.Mesh;
  skinMeshes: THREE.Mesh[];
  legMeshes: THREE.Mesh[];
  mantle: THREE.Group;
  weapon: THREE.Group;
  walkPhase: number;
  lastAnimationPosition: Vec2 | null;
  lastAnimationTimeMs: number | null;
  basicAttackStartedAtMs: number | null;
  previousBasicAttackCooldownMs: number;
};

type CharacterVisualAppearance = {
  race: CharacterRace;
  baseClass: BaseClass;
  sex: CharacterSex;
  hairStyle: AppearanceOptionIndex;
  hairColor: AppearanceOptionIndex;
  face: AppearanceOptionIndex;
};

const CAMERA_OFFSET = new THREE.Vector3(-18, 22, 18);
const INITIAL_CAMERA_DISTANCE = CAMERA_OFFSET.length();
const INITIAL_CAMERA_ELEVATION = Math.asin(CAMERA_OFFSET.y / INITIAL_CAMERA_DISTANCE);
const INITIAL_CAMERA_YAW = Math.atan2(CAMERA_OFFSET.z, CAMERA_OFFSET.x);
const CAMERA_MIN_DISTANCE = 24;
const CAMERA_MAX_DISTANCE = 54;
const CAMERA_ORBIT_SENSITIVITY = 0.008;
const CAMERA_ZOOM_SENSITIVITY = 0.012;
const BASIC_ATTACK_VISUAL_MS = 420;

const toWorld = (point: Vec2, y = 0): THREE.Vector3 => new THREE.Vector3(point.x, y, point.z);

const createCircle = (radius: number, color: string, opacity = 1): THREE.Mesh => {
  const geometry = new THREE.RingGeometry(radius * 0.82, radius, 32);
  const material = new THREE.MeshBasicMaterial({
    color,
    transparent: true,
    opacity,
    side: THREE.DoubleSide,
  });
  const mesh = new THREE.Mesh(geometry, material);
  mesh.rotation.x = -Math.PI / 2;
  mesh.position.y = 0.05;
  return mesh;
};

const createPathLine = (color: string, opacity: number): THREE.Line => {
  const material = new THREE.LineBasicMaterial({
    color,
    transparent: true,
    opacity,
  });
  const geometry = new THREE.BufferGeometry().setFromPoints([new THREE.Vector3(), new THREE.Vector3()]);
  const line = new THREE.Line(geometry, material);
  line.visible = false;
  return line;
};

const SKIN_COLORS: Record<CharacterRace, readonly [string, string, string]> = {
  Human: ['#d8b99f', '#cfaa91', '#e0c7ad'],
  Elf: ['#e5d2b5', '#dfc9a7', '#f0dfc0'],
  'Dark Elf': ['#8c8da5', '#767992', '#9da0b6'],
  Orc: ['#78906d', '#6d8664', '#879b75'],
  Dwarf: ['#c0926f', '#ad7c58', '#d3a27b'],
};

const HAIR_COLORS: Record<CharacterRace, readonly [string, string, string]> = {
  Human: ['#6b4e37', '#c5a46a', '#26211c'],
  Elf: ['#e4d47d', '#fff0a8', '#c6c3b4'],
  'Dark Elf': ['#e7eef0', '#c5d4dc', '#f4f4ff'],
  Orc: ['#1d201c', '#44372d', '#2e3026'],
  Dwarf: ['#b66d3d', '#e0a65d', '#f1f1e6'],
};

const requireAppearanceIndex = (value: number, field: string): AppearanceOptionIndex => {
  if (value === 0 || value === 1 || value === 2) {
    return value;
  }
  throw new Error(`Invalid canonical ${field}: ${value}`);
};

const gearColorFor = (appearance: CharacterVisualAppearance): string => {
  if (appearance.baseClass === 'Mage') {
    return appearance.race === 'Dark Elf' ? '#35405e' : '#52617b';
  }
  return appearance.race === 'Orc' ? '#4e5948' : '#5a5f67';
};

const legColorFor = (appearance: CharacterVisualAppearance): string =>
  appearance.baseClass === 'Mage' ? '#20283d' : '#26252a';

const characterScaleFor = (appearance: CharacterVisualAppearance): THREE.Vector3 => {
  const isFemale = appearance.sex === 'Female';
  if (appearance.race === 'Dwarf') {
    return new THREE.Vector3(isFemale ? 0.88 : 0.95, isFemale ? 0.82 : 0.88, isFemale ? 0.88 : 0.95);
  }
  if (appearance.race === 'Orc') {
    return new THREE.Vector3(isFemale ? 1.08 : 1.18, isFemale ? 1.04 : 1.13, isFemale ? 1.08 : 1.18);
  }
  if (isFemale) {
    return new THREE.Vector3(0.9, 0.96, 0.9);
  }
  return new THREE.Vector3(1, 1, 1);
};

type ColorableMaterial = THREE.Material & { color: THREE.Color };

const hasMaterialColor = (material: THREE.Material): material is ColorableMaterial =>
  'color' in material && material.color instanceof THREE.Color;

const setMeshColor = (mesh: THREE.Mesh, color: string): void => {
  const material = mesh.material;
  if (Array.isArray(material)) {
    for (const entry of material) {
      if (hasMaterialColor(entry)) {
        entry.color.set(color);
      }
    }
    return;
  }
  if (hasMaterialColor(material)) {
    material.color.set(color);
  }
};

const createColumn = (height: number, color: string): THREE.Group => {
  const group = new THREE.Group();
  const base = new THREE.Mesh(
    new THREE.CylinderGeometry(1.2, 1.6, 0.8, 6),
    new THREE.MeshStandardMaterial({ color: '#4a4750', roughness: 0.95 }),
  );
  const shaft = new THREE.Mesh(
    new THREE.CylinderGeometry(0.65, 0.85, height, 6),
    new THREE.MeshStandardMaterial({ color, roughness: 0.9 }),
  );
  shaft.position.y = height / 2;
  group.add(base, shaft);
  return group;
};

const createMobVisual = (tint: string): MobVisual => {
  const group = new THREE.Group();

  const body = new THREE.Mesh(
    new THREE.SphereGeometry(1.25, 14, 12),
    new THREE.MeshStandardMaterial({ color: tint, roughness: 0.85 }),
  );
  body.position.y = 1.7;

  const head = new THREE.Mesh(
    new THREE.SphereGeometry(0.78, 12, 10),
    new THREE.MeshStandardMaterial({ color: '#d4c5d2', roughness: 0.75 }),
  );
  head.position.set(0.85, 2.4, 0);

  const hornLeft = new THREE.Mesh(
    new THREE.ConeGeometry(0.18, 0.8, 4),
    new THREE.MeshStandardMaterial({ color: '#2c1f2f', roughness: 1 }),
  );
  hornLeft.position.set(1.15, 2.95, 0.25);
  hornLeft.rotation.z = -0.9;

  const hornRight = hornLeft.clone();
  hornRight.position.z = -0.25;

  const legGeometry = new THREE.CylinderGeometry(0.15, 0.2, 1.2, 5);
  const legMaterial = new THREE.MeshStandardMaterial({ color: '#2e2530', roughness: 1 });
  for (const [x, z] of [
    [-0.55, -0.4],
    [-0.55, 0.4],
    [0.35, -0.4],
    [0.35, 0.4],
  ]) {
    const leg = new THREE.Mesh(legGeometry, legMaterial);
    leg.position.set(x, 0.7, z);
    group.add(leg);
  }

  const hpBar = new THREE.Mesh(
    new THREE.BoxGeometry(2, 0.16, 0.12),
    new THREE.MeshBasicMaterial({ color: '#d85c60' }),
  );
  hpBar.position.set(0, 3.8, 0);

  group.add(body, head, hornLeft, hornRight, hpBar);
  return { group, hpBar };
};

const createNpcVisual = (): THREE.Group => {
  const group = new THREE.Group();
  const robe = new THREE.Mesh(
    new THREE.CylinderGeometry(0.72, 0.95, 2.7, 6),
    new THREE.MeshStandardMaterial({ color: '#3a546f', roughness: 0.9 }),
  );
  robe.position.y = 1.4;

  const head = new THREE.Mesh(
    new THREE.SphereGeometry(0.48, 10, 10),
    new THREE.MeshStandardMaterial({ color: '#d4c2ad', roughness: 0.75 }),
  );
  head.position.y = 2.95;

  const staff = new THREE.Mesh(
    new THREE.CylinderGeometry(0.07, 0.07, 3.5, 6),
    new THREE.MeshStandardMaterial({ color: '#8f6a46', roughness: 0.9 }),
  );
  staff.position.set(0.72, 1.7, 0);
  staff.rotation.z = 0.15;

  group.add(robe, head, staff);
  return group;
};

const createLootVisual = (): THREE.Group => {
  const group = new THREE.Group();
  const stone = new THREE.Mesh(
    new THREE.OctahedronGeometry(0.58, 0),
    new THREE.MeshStandardMaterial({ color: '#f0c774', emissive: '#5a410f', roughness: 0.45 }),
  );
  stone.position.y = 0.65;
  group.add(stone);
  return group;
};

const createPlayerVisual = (): PlayerVisual => {
  const group = new THREE.Group();
  const proceduralRoot = new THREE.Group();

  const legMaterial = new THREE.MeshStandardMaterial({ color: '#26252a', roughness: 0.95 });
  const skinMaterial = new THREE.MeshStandardMaterial({ color: '#d8c1b3', roughness: 0.82 });
  const torsoMaterial = new THREE.MeshStandardMaterial({ color: '#6a6d82', roughness: 0.85 });
  const hairMaterial = new THREE.MeshStandardMaterial({ color: '#6b4e37', roughness: 0.84 });

  const torso = new THREE.Mesh(new THREE.BoxGeometry(1.1, 1.55, 0.8), torsoMaterial);
  torso.position.y = 2.45;

  const hip = new THREE.Mesh(new THREE.BoxGeometry(0.86, 0.65, 0.72), torsoMaterial);
  hip.position.y = 1.45;

  const head = new THREE.Mesh(new THREE.SphereGeometry(0.45, 12, 10), skinMaterial);
  head.position.y = 3.65;

  const hair = new THREE.Mesh(new THREE.SphereGeometry(0.47, 12, 8, 0, Math.PI * 2, 0, Math.PI * 0.54), hairMaterial);
  hair.position.set(0, 3.78, -0.02);

  const faceMark = new THREE.Mesh(new THREE.BoxGeometry(0.2, 0.035, 0.025), new THREE.MeshBasicMaterial({ color: '#17120f' }));
  faceMark.position.set(0, 3.52, 0.43);

  const armGeometry = new THREE.BoxGeometry(0.28, 1.1, 0.28);
  const armLeft = new THREE.Mesh(armGeometry, skinMaterial);
  armLeft.position.set(-0.8, 2.4, 0);
  const armRight = armLeft.clone();
  armRight.position.x = 0.8;

  const legGeometry = new THREE.BoxGeometry(0.35, 1.2, 0.35);
  const legLeft = new THREE.Mesh(legGeometry, legMaterial);
  legLeft.position.set(-0.26, 0.6, 0);
  const legRight = legLeft.clone();
  legRight.position.x = 0.26;

  const mantle = new THREE.Group();
  const shoulderLeft = new THREE.Mesh(
    new THREE.BoxGeometry(0.55, 0.28, 0.7),
    new THREE.MeshStandardMaterial({ color: '#5f8ccf', roughness: 0.78 }),
  );
  shoulderLeft.position.set(-0.72, 2.95, 0);
  const shoulderRight = shoulderLeft.clone();
  shoulderRight.position.x = 0.72;
  const mantleBack = new THREE.Mesh(
    new THREE.BoxGeometry(1.05, 1.45, 0.18),
    new THREE.MeshStandardMaterial({ color: '#3f5c90', roughness: 0.84 }),
  );
  mantleBack.position.set(0, 2.15, -0.48);
  mantle.add(shoulderLeft, shoulderRight, mantleBack);
  mantle.visible = false;

  const weapon = new THREE.Group();
  const shaft = new THREE.Mesh(
    new THREE.CylinderGeometry(0.06, 0.06, 2.6, 6),
    new THREE.MeshStandardMaterial({ color: '#bda178', roughness: 0.9 }),
  );
  shaft.rotation.z = 0.1;
  shaft.position.set(0.8, 1.8, 0);
  const tip = new THREE.Mesh(
    new THREE.ConeGeometry(0.18, 0.75, 5),
    new THREE.MeshStandardMaterial({ color: '#d1d6dc', roughness: 0.4, metalness: 0.2 }),
  );
  tip.rotation.z = -Math.PI / 2;
  tip.position.set(0.8, 2.95, 0);
  weapon.add(shaft, tip);
  weapon.visible = false;

  proceduralRoot.add(torso, hip, head, hair, faceMark, armLeft, armRight, legLeft, legRight, mantle, weapon);
  group.add(proceduralRoot);
  return {
    group,
    proceduralRoot,
    torso,
    hip,
    head,
    hair,
    faceMark,
    armLeft,
    armRight,
    legLeft,
    legRight,
    skinMeshes: [head, armLeft, armRight],
    legMeshes: [legLeft, legRight],
    mantle,
    weapon,
    walkPhase: 0,
    lastAnimationPosition: null,
    lastAnimationTimeMs: null,
    basicAttackStartedAtMs: null,
    previousBasicAttackCooldownMs: 0,
  };
};

const animatePlayerVisual = (
  visual: PlayerVisual,
  timeMs: number,
  position: Vec2,
  options: {
    movingHint?: boolean;
    casting?: boolean;
    dead?: boolean;
    basicAttackCooldownMs?: number;
  } = {},
): void => {
  const deltaMs = visual.lastAnimationTimeMs === null ? 16 : Math.max(0, Math.min(timeMs - visual.lastAnimationTimeMs, 80));
  const previousPosition = visual.lastAnimationPosition;
  const movedDistance = previousPosition
    ? Math.hypot(position.x - previousPosition.x, position.z - previousPosition.z)
    : 0;
  const speed = deltaMs > 0 ? movedDistance / (deltaMs / 1000) : 0;
  const moving = movedDistance > 0.002 || options.movingHint === true;
  visual.lastAnimationPosition = { ...position };
  visual.lastAnimationTimeMs = timeMs;

  const basicCooldown = options.basicAttackCooldownMs ?? 0;
  if (basicCooldown > visual.previousBasicAttackCooldownMs + 160) {
    visual.basicAttackStartedAtMs = timeMs;
  }
  visual.previousBasicAttackCooldownMs = basicCooldown;

  if (moving) {
    visual.walkPhase += deltaMs * THREE.MathUtils.clamp(0.0075 + speed * 0.0025, 0.0075, 0.024);
  } else {
    visual.walkPhase += deltaMs * 0.0012;
  }

  visual.proceduralRoot.rotation.set(0, 0, 0);
  visual.torso.rotation.set(0, 0, 0);
  visual.hip.rotation.set(0, 0, 0);
  visual.head.rotation.set(0, 0, 0);
  visual.armLeft.rotation.set(0, 0, 0.1);
  visual.armRight.rotation.set(0, 0, -0.1);
  visual.legLeft.rotation.set(0, 0, 0);
  visual.legRight.rotation.set(0, 0, 0);
  visual.weapon.rotation.set(0, 0, 0);

  if (options.dead) {
    visual.proceduralRoot.rotation.z = Math.PI * 0.5;
    visual.proceduralRoot.position.y = 0.72;
    visual.group.position.y = 0;
    return;
  }
  visual.proceduralRoot.position.y = 0;

  const idle = Math.sin(timeMs * 0.0021 + visual.group.id * 0.17);
  const walkSwing = Math.sin(visual.walkPhase);
  const walkCounterSwing = Math.cos(visual.walkPhase * 2);
  const walkAmount = moving ? 1 : 0;
  const idleAmount = moving ? 0 : 1;

  visual.group.position.y = moving ? Math.abs(walkCounterSwing) * 0.045 : idle * 0.026;
  visual.torso.rotation.z = walkCounterSwing * 0.026 * walkAmount + idle * 0.012 * idleAmount;
  visual.hip.rotation.z = -visual.torso.rotation.z * 0.55;
  visual.head.position.y = 3.65 + idle * 0.024 * idleAmount;
  visual.hair.position.y += idle * 0.022 * idleAmount;
  visual.faceMark.position.y = 3.52 + idle * 0.018 * idleAmount;

  visual.legLeft.rotation.x = -walkSwing * 0.58 * walkAmount;
  visual.legRight.rotation.x = walkSwing * 0.58 * walkAmount;
  visual.armLeft.rotation.x = walkSwing * 0.62 * walkAmount;
  visual.armRight.rotation.x = -walkSwing * 0.62 * walkAmount;

  if (options.casting) {
    const pulse = Math.sin(timeMs * 0.011) * 0.08;
    visual.torso.rotation.x = pulse * 0.28;
    visual.armLeft.rotation.x = -0.82 + pulse;
    visual.armRight.rotation.x = -0.82 - pulse;
    visual.armLeft.rotation.z = 0.5;
    visual.armRight.rotation.z = -0.5;
    visual.weapon.rotation.x = -0.32 + pulse;
    return;
  }

  if (visual.basicAttackStartedAtMs !== null) {
    const progress = (timeMs - visual.basicAttackStartedAtMs) / BASIC_ATTACK_VISUAL_MS;
    if (progress >= 1) {
      visual.basicAttackStartedAtMs = null;
      return;
    }
    const slash = Math.sin(progress * Math.PI);
    const recover = 1 - progress;
    visual.torso.rotation.y = -0.26 * slash;
    visual.torso.rotation.z += 0.16 * slash * recover;
    visual.armRight.rotation.x = -1.18 * slash;
    visual.armRight.rotation.z = -0.35 - 0.35 * slash;
    visual.armLeft.rotation.x = 0.32 * slash;
    visual.weapon.rotation.x = -0.78 * slash;
    visual.weapon.rotation.z = -0.28 * slash;
  }
};

const applyPlayerVisualAppearance = (
  visual: PlayerVisual,
  appearance: CharacterVisualAppearance,
  options: { dead?: boolean } = {},
): void => {
  const face = requireAppearanceIndex(appearance.face, 'face');
  const hairStyle = requireAppearanceIndex(appearance.hairStyle, 'hairStyle');
  const hairColor = requireAppearanceIndex(appearance.hairColor, 'hairColor');
  const scale = characterScaleFor(appearance);
  visual.group.scale.copy(scale);
  for (const mesh of visual.skinMeshes) {
    setMeshColor(mesh, SKIN_COLORS[appearance.race][face]);
  }
  setMeshColor(visual.hair, HAIR_COLORS[appearance.race][hairColor]);
  setMeshColor(visual.torso, options.dead ? '#63505a' : gearColorFor(appearance));
  setMeshColor(visual.hip, options.dead ? '#4c4348' : gearColorFor(appearance));
  for (const mesh of visual.legMeshes) {
    setMeshColor(mesh, options.dead ? '#3c3438' : legColorFor(appearance));
  }
  visual.hair.scale.set(1 + hairStyle * 0.08, 0.92 + hairStyle * 0.08, 1 + hairStyle * 0.04);
  visual.hair.position.y = 3.78 - hairStyle * 0.035;
  visual.faceMark.scale.set(0.72 + face * 0.24, 1, 1);
};

export class Scene3D {
  private readonly store: GameStore;
  private readonly interactive: boolean;
  private readonly onMoveIntent?: (point: Vec2) => void;
  private readonly onSelectTarget?: (targetId: string) => void;
  private readonly onInteractNpc?: (npcId: string) => void;
  private readonly onPickUpLoot?: (lootId: string) => void;
  private readonly root: HTMLDivElement;
  private readonly rendererHost: HTMLDivElement;
  private readonly labelsHost: HTMLDivElement;
  private readonly scene = new THREE.Scene();
  private readonly camera = new THREE.PerspectiveCamera(48, 1, 0.1, 250);
  private readonly renderer: THREE.WebGLRenderer;
  private readonly raycaster = new THREE.Raycaster();
  private readonly pointer = new THREE.Vector2();
  private readonly player = createPlayerVisual();
  private readonly targetRing = createCircle(1.8, '#ffd76e', 0.95);
  private readonly destinationMarker = createCircle(0.75, '#8dd9ff', 0.85);
  private readonly pendingPathLine = createPathLine('#8dd9ff', 0.6);
  private readonly authoritativePathLine = createPathLine('#ffe17b', 0.85);
  private readonly aoePreview = createCircle(4.8, '#b78cff', 0.28);
  private readonly otherPlayerVisuals = new Map<string, PlayerVisual>();
  private readonly mobVisuals = new Map<string, MobVisual>();
  private readonly companionVisuals = new Map<string, CompanionVisual>();
  private readonly lootVisuals = new Map<string, THREE.Group>();
  private readonly npcVisuals = new Map<string, THREE.Group>();
  private readonly groundMesh: THREE.Mesh;
  private readonly clickables: Object3D[] = [];
  private readonly labelNodes = new Map<string, HTMLDivElement>();
  private cameraYaw = INITIAL_CAMERA_YAW;
  private cameraDistance = INITIAL_CAMERA_DISTANCE;
  private isCameraOrbiting = false;
  private lastOrbitPointerX = 0;

  constructor(
    container: HTMLElement,
    store: GameStore,
    options?: {
      interactive?: boolean;
      onMoveIntent?: (point: Vec2) => void;
      onSelectTarget?: (targetId: string) => void;
      onInteractNpc?: (npcId: string) => void;
      onPickUpLoot?: (lootId: string) => void;
    },
  ) {
    this.store = store;
    this.interactive = options?.interactive ?? true;
    this.onMoveIntent = options?.onMoveIntent;
    this.onSelectTarget = options?.onSelectTarget;
    this.onInteractNpc = options?.onInteractNpc;
    this.onPickUpLoot = options?.onPickUpLoot;
    this.root = document.createElement('div');
    this.root.className = 'scene-root';
    this.rendererHost = document.createElement('div');
    this.rendererHost.className = 'scene-canvas';
    this.labelsHost = document.createElement('div');
    this.labelsHost.className = 'floating-layer';
    this.root.append(this.rendererHost, this.labelsHost);
    container.append(this.root);

    this.renderer = new THREE.WebGLRenderer({ antialias: true });
    this.renderer.setPixelRatio(Math.min(window.devicePixelRatio, 2));
    this.renderer.shadowMap.enabled = true;
    this.renderer.shadowMap.type = THREE.PCFSoftShadowMap;
    this.rendererHost.appendChild(this.renderer.domElement);

    this.scene.fog = new THREE.Fog('#090b12', 34, 120);
    this.camera.position.copy(CAMERA_OFFSET);

    const ambient = new THREE.AmbientLight('#6881a9', 1.8);
    const sun = new THREE.DirectionalLight('#ced9ff', 2.8);
    sun.position.set(-22, 34, 12);
    sun.castShadow = true;
    sun.shadow.mapSize.set(1024, 1024);
    sun.shadow.camera.near = 1;
    sun.shadow.camera.far = 90;
    sun.shadow.camera.left = -45;
    sun.shadow.camera.right = 45;
    sun.shadow.camera.top = 45;
    sun.shadow.camera.bottom = -45;
    this.scene.add(ambient, sun);

    const ground = new THREE.Group();
    ground.add(
      new THREE.Mesh(
        new THREE.BoxGeometry(38, 0.8, 34),
        new THREE.MeshStandardMaterial({ color: '#292c37', roughness: 0.98 }),
      ),
      new THREE.Mesh(
        new THREE.BoxGeometry(18, 0.75, 34),
        new THREE.MeshStandardMaterial({ color: '#2b3036', roughness: 0.96 }),
      ),
      new THREE.Mesh(
        new THREE.BoxGeometry(40, 0.72, 34),
        new THREE.MeshStandardMaterial({ color: '#233329', roughness: 0.98 }),
      ),
      new THREE.Mesh(
        new THREE.BoxGeometry(34, 0.7, 34),
        new THREE.MeshStandardMaterial({ color: '#2e2628', roughness: 0.97 }),
      ),
    );
    ground.children[0].position.set(-1, -0.4, 0);
    ground.children[1].position.set(27, -0.41, 0);
    ground.children[2].position.set(56, -0.42, 0);
    ground.children[3].position.set(83, -0.43, 0);
    this.scene.add(ground);

    const road = new THREE.Mesh(
      new THREE.BoxGeometry(22, 0.05, 5.5),
      new THREE.MeshStandardMaterial({ color: '#6d675c', roughness: 0.96 }),
    );
    road.position.set(17, 0.03, 0);
    this.scene.add(road);

    const gateLeft = createColumn(5.8, '#4d4555');
    gateLeft.position.set(18, 0, -4.8);
    const gateRight = createColumn(5.8, '#4d4555');
    gateRight.position.set(18, 0, 4.8);
    const gateBeam = new THREE.Mesh(
      new THREE.BoxGeometry(0.95, 0.9, 10.6),
      new THREE.MeshStandardMaterial({ color: '#655c70', roughness: 0.88 }),
    );
    gateBeam.position.set(18, 5.55, 0);
    this.scene.add(gateLeft, gateRight, gateBeam);

    const obelisk = createColumn(7.4, '#5d6070');
    obelisk.position.set(-6, 0, -6);
    this.scene.add(obelisk);

    const merchantStand = new THREE.Group();
    const table = new THREE.Mesh(
      new THREE.BoxGeometry(3.4, 0.4, 1.8),
      new THREE.MeshStandardMaterial({ color: '#73573d', roughness: 0.95 }),
    );
    table.position.y = 1.25;
    const awning = new THREE.Mesh(
      new THREE.ConeGeometry(2.4, 1.45, 4),
      new THREE.MeshStandardMaterial({ color: '#6e4861', roughness: 0.86 }),
    );
    awning.position.y = 3.2;
    awning.rotation.y = Math.PI * 0.25;
    merchantStand.add(table, awning);
    merchantStand.position.set(-10, 0, 8);
    this.scene.add(merchantStand);

    for (const [x, z] of [
      [62, -10],
      [69, 10],
      [82, -11],
      [89, 8],
    ]) {
      const ruin = createColumn(3.4, '#59505a');
      ruin.scale.set(0.7, 0.7, 0.7);
      ruin.position.set(x, 0, z);
      this.scene.add(ruin);
    }

    this.groundMesh = new THREE.Mesh(
      new THREE.PlaneGeometry(140, 90),
      new THREE.MeshBasicMaterial({ visible: false }),
    );
    this.groundMesh.rotation.x = -Math.PI / 2;
    this.groundMesh.position.y = 0.01;
    this.scene.add(this.groundMesh);

    this.player.group.castShadow = true;
    this.player.group.receiveShadow = true;
    this.scene.add(
      this.player.group,
      this.targetRing,
      this.destinationMarker,
      this.pendingPathLine,
      this.authoritativePathLine,
      this.aoePreview,
    );
    this.targetRing.visible = false;
    this.destinationMarker.visible = false;
    this.pendingPathLine.visible = false;
    this.authoritativePathLine.visible = false;
    this.aoePreview.visible = false;

    this.createActors(this.store.getState());
    this.handlePointerDown = this.handlePointerDown.bind(this);
    this.handleCameraPointerMove = this.handleCameraPointerMove.bind(this);
    this.handleCameraPointerUp = this.handleCameraPointerUp.bind(this);
    this.handleContextMenu = this.handleContextMenu.bind(this);
    this.handleWheel = this.handleWheel.bind(this);
    this.handleResize = this.handleResize.bind(this);
    if (this.interactive) {
      this.renderer.domElement.addEventListener('pointerdown', this.handlePointerDown);
      this.renderer.domElement.addEventListener('contextmenu', this.handleContextMenu);
      this.renderer.domElement.addEventListener('wheel', this.handleWheel, { passive: false });
    }
    window.addEventListener('resize', this.handleResize);
    this.handleResize();
  }

  private createActors(state: GameState): void {
    for (const otherPlayer of Object.values(state.otherPlayers)) {
      this.ensureOtherPlayerVisual(otherPlayer);
    }

    for (const mob of Object.values(state.mobs)) {
      this.ensureMobVisual(mob);
    }

    for (const companion of Object.values(state.companions)) {
      this.ensureCompanionVisual(companion);
    }

    for (const npc of Object.values(state.npcs)) {
      const visual = createNpcVisual();
      visual.userData = { kind: 'npc', id: npc.id } satisfies ClickTarget;
      this.npcVisuals.set(npc.id, visual);
      this.scene.add(visual);
      this.clickables.push(visual);
    }
  }

  private ensureMobVisual(mob: GameState['mobs'][string]): MobVisual {
    let visual = this.mobVisuals.get(mob.id);
    if (visual) {
      return visual;
    }
    visual = createMobVisual(gameTemplates.mobTemplates[mob.templateId].tint);
    visual.group.userData = { kind: 'mob', id: mob.id } satisfies ClickTarget;
    visual.group.castShadow = true;
    visual.group.receiveShadow = true;
    this.mobVisuals.set(mob.id, visual);
    this.scene.add(visual.group);
    this.clickables.push(visual.group);
    return visual;
  }

  private ensureCompanionVisual(companion: GameState['companions'][string]): CompanionVisual | null {
    const existing = this.companionVisuals.get(companion.id);
    if (existing) {
      return existing;
    }
    const template = gameTemplates.mobTemplates[companion.visualTemplateId];
    if (!template) {
      return null;
    }
    const visual = createMobVisual(template.tint);
    visual.group.scale.setScalar(companion.mounted ? 0.78 : 0.68);
    visual.group.castShadow = true;
    visual.group.receiveShadow = true;
    this.companionVisuals.set(companion.id, visual);
    this.scene.add(visual.group);
    return visual;
  }

  private ensureOtherPlayerVisual(otherPlayer: GameState['otherPlayers'][string]): PlayerVisual {
    let visual = this.otherPlayerVisuals.get(otherPlayer.id);
    if (visual) {
      return visual;
    }
    visual = createPlayerVisual();
    visual.weapon.visible = false;
    visual.mantle.visible = false;
    visual.group.castShadow = true;
    visual.group.receiveShadow = true;
    this.otherPlayerVisuals.set(otherPlayer.id, visual);
    this.scene.add(visual.group);
    return visual;
  }

  private handlePointerDown(event: PointerEvent): void {
    if (event.button === 2) {
      this.startCameraOrbit(event);
      return;
    }
    if (event.button !== 0) {
      return;
    }
    this.handlePointerAt(event.clientX, event.clientY);
  }

  private startCameraOrbit(event: PointerEvent): void {
    event.preventDefault();
    event.stopPropagation();
    this.isCameraOrbiting = true;
    this.lastOrbitPointerX = event.clientX;
    this.renderer.domElement.setPointerCapture?.(event.pointerId);
    this.renderer.domElement.addEventListener('pointermove', this.handleCameraPointerMove);
    this.renderer.domElement.addEventListener('pointerup', this.handleCameraPointerUp, { once: true });
    this.renderer.domElement.addEventListener('pointercancel', this.handleCameraPointerUp, { once: true });
  }

  private handleCameraPointerMove(event: PointerEvent): void {
    if (!this.isCameraOrbiting) {
      return;
    }
    const deltaX = event.clientX - this.lastOrbitPointerX;
    this.lastOrbitPointerX = event.clientX;
    this.cameraYaw += deltaX * CAMERA_ORBIT_SENSITIVITY;
    event.preventDefault();
  }

  private handleCameraPointerUp(event: PointerEvent): void {
    this.isCameraOrbiting = false;
    if (this.renderer.domElement.hasPointerCapture?.(event.pointerId)) {
      this.renderer.domElement.releasePointerCapture(event.pointerId);
    }
    this.renderer.domElement.removeEventListener('pointermove', this.handleCameraPointerMove);
    this.renderer.domElement.removeEventListener('pointerup', this.handleCameraPointerUp);
    this.renderer.domElement.removeEventListener('pointercancel', this.handleCameraPointerUp);
    event.preventDefault();
  }

  private handleContextMenu(event: MouseEvent): void {
    event.preventDefault();
  }

  private handleWheel(event: WheelEvent): void {
    event.preventDefault();
    this.cameraDistance = THREE.MathUtils.clamp(
      this.cameraDistance - event.deltaY * CAMERA_ZOOM_SENSITIVITY,
      CAMERA_MIN_DISTANCE,
      CAMERA_MAX_DISTANCE,
    );
  }

  private handlePointerAt(clientX: number, clientY: number): void {
    const bounds = this.renderer.domElement.getBoundingClientRect();
    this.pointer.x = ((clientX - bounds.left) / bounds.width) * 2 - 1;
    this.pointer.y = -((clientY - bounds.top) / bounds.height) * 2 + 1;
    this.raycaster.setFromCamera(this.pointer, this.camera);

    const clickableHits: ClickHit[] = this.raycaster
      .intersectObjects(this.clickables, true)
      .flatMap((entry) => {
        const target = this.resolveClickTarget(entry.object);
        return target ? [{ target, distance: entry.distance }] : [];
      });
    const clickable = selectClickTarget(clickableHits);
    if (clickable) {
      if (clickable.kind === 'mob') {
        if (this.onSelectTarget) {
          this.onSelectTarget(clickable.id);
        } else {
          this.store.dispatch({ type: 'selectTarget', targetId: clickable.id });
        }
      } else if (clickable.kind === 'loot') {
        if (this.onPickUpLoot) {
          this.onPickUpLoot(clickable.id);
          return;
        }
        if (this.onMoveIntent || this.onSelectTarget) {
          return;
        }
        this.store.dispatch({ type: 'pickUpLoot', lootId: clickable.id });
      } else if (clickable.kind === 'npc') {
        if (this.onInteractNpc) {
          this.onInteractNpc(clickable.id);
          return;
        }
        if (this.onMoveIntent || this.onSelectTarget) {
          return;
        }
        this.store.dispatch({ type: 'interactNpc', npcId: clickable.id });
      }
      return;
    }

    const terrainHit = this.raycaster.intersectObject(this.groundMesh);
    if (!terrainHit[0]) {
      return;
    }

    const point = terrainHit[0].point;
    const clampedPoint = { x: THREE.MathUtils.clamp(point.x, -18, 97), z: THREE.MathUtils.clamp(point.z, -16, 16) };
    if (this.onMoveIntent) {
      this.onMoveIntent(clampedPoint);
      return;
    }
    this.store.dispatch({
      type: 'moveToPoint',
      point: clampedPoint,
    });
  }

  screenPointForWorld(point: Vec2): { x: number; y: number } {
    const projected = toWorld(point, 1.1).project(this.camera);
    return {
      x: ((projected.x + 1) / 2) * this.root.clientWidth,
      y: ((-projected.y + 1) / 2) * this.root.clientHeight,
    };
  }

  simulateCanvasClick(point: Vec2): void {
    const screen = this.screenPointForWorld(point);
    this.handlePointerAt(screen.x, screen.y);
  }

  private resolveClickTarget(object: Object3D | null): ClickTarget | null {
    let current: Object3D | null = object;
    while (current) {
      const data = current.userData as Partial<ClickTarget>;
      if (data.kind && data.id) {
        return data as ClickTarget;
      }
      current = current.parent;
    }
    return null;
  }

  private handleResize(): void {
    const { clientWidth, clientHeight } = this.root;
    this.camera.aspect = Math.max(clientWidth / Math.max(clientHeight, 1), 0.1);
    this.camera.updateProjectionMatrix();
    this.renderer.setSize(clientWidth, clientHeight, false);
  }

  update(state: GameState): void {
    this.player.group.position.copy(toWorld(state.player.position));
    this.player.group.rotation.y = -state.player.facing + Math.PI * 0.5;
    applyPlayerVisualAppearance(this.player, state.player);
    animatePlayerVisual(this.player, state.timeMs, state.player.position, {
      movingHint: Boolean(state.player.moveTarget),
      casting: Boolean(state.player.cast),
      dead: Boolean(state.player.deadUntilMs),
      basicAttackCooldownMs: state.player.cooldowns.basic_attack ?? 0,
    });

    const gear = getEquippedBySlot(state);
    this.player.weapon.visible = Boolean(gear.weapon);
    this.player.mantle.visible = Boolean(gear.chest);
    if (gear.chest) {
      const tint = getTemplate(gear.chest.templateId).appearance?.tint ?? '#5f8ccf';
      for (const child of this.player.mantle.children) {
        ((child as THREE.Mesh).material as THREE.MeshStandardMaterial).color.set(tint);
      }
    }

    this.destinationMarker.visible = Boolean(state.destinationMarker);
    if (state.destinationMarker) {
      this.destinationMarker.position.copy(toWorld(state.destinationMarker, 0.08));
      this.destinationMarker.rotation.z += 0.01;
    }
    this.updatePathLine(this.pendingPathLine, state.pendingPath, 0.14);
    this.updatePathLine(this.authoritativePathLine, state.authoritativePath, 0.18);

    const cast = state.player.cast ? gameTemplates.skills[state.player.cast.skillId] : null;
    const target = getTargetMob(state);
    this.targetRing.visible = Boolean(target);
    this.aoePreview.visible = Boolean(target && cast?.targetType === 'target_centered_aoe');
    if (target) {
      this.targetRing.position.copy(toWorld(target.position, 0.09));
      if (cast?.targetType === 'target_centered_aoe') {
        this.aoePreview.position.copy(toWorld(target.position, 0.12));
      }
    }

    for (const mob of Object.values(state.mobs)) {
      this.ensureMobVisual(mob);
    }

    for (const otherPlayer of Object.values(state.otherPlayers)) {
      this.ensureOtherPlayerVisual(otherPlayer);
    }

    for (const companion of Object.values(state.companions)) {
      this.ensureCompanionVisual(companion);
    }

    for (const otherPlayerId of Array.from(this.otherPlayerVisuals.keys())) {
      if (state.otherPlayers[otherPlayerId]) {
        continue;
      }
      const visual = this.otherPlayerVisuals.get(otherPlayerId);
      if (!visual) {
        continue;
      }
      this.scene.remove(visual.group);
      this.otherPlayerVisuals.delete(otherPlayerId);
    }

    for (const [otherPlayerId, visual] of this.otherPlayerVisuals) {
      const otherPlayer = state.otherPlayers[otherPlayerId];
      if (!otherPlayer) {
        continue;
      }
      visual.group.visible = true;
      visual.group.position.copy(toWorld(otherPlayer.position));
      visual.group.rotation.y = -otherPlayer.facing + Math.PI * 0.5;
      applyPlayerVisualAppearance(visual, otherPlayer, { dead: otherPlayer.dead });
      animatePlayerVisual(visual, state.timeMs, otherPlayer.position, { dead: otherPlayer.dead });
    }

    for (const companionId of Array.from(this.companionVisuals.keys())) {
      if (state.companions[companionId]) {
        continue;
      }
      const visual = this.companionVisuals.get(companionId);
      if (!visual) {
        continue;
      }
      this.scene.remove(visual.group);
      this.companionVisuals.delete(companionId);
    }

    for (const [companionId, visual] of this.companionVisuals) {
      const companion = state.companions[companionId];
      if (!companion) {
        continue;
      }
      const template = gameTemplates.mobTemplates[companion.visualTemplateId];
      const ownerFacing =
        companion.ownerId === state.player.id ? state.player.facing : state.otherPlayers[companion.ownerId]?.facing ?? 0;
      visual.group.visible = true;
      visual.group.scale.setScalar(companion.mounted ? 0.78 : 0.68);
      visual.group.position.copy(toWorld(companion.position));
      visual.group.rotation.y = companion.mounted
        ? -ownerFacing + Math.PI * 0.5
        : Math.sin(state.timeMs * 0.001 + companionId.length) * 0.06;
      const maxHp = template?.maxHp ?? 1;
      visual.hpBar.scale.x = 1;
      visual.hpBar.position.x = 0;
      visual.hpBar.visible = !companion.mounted && maxHp > 0;
    }

    for (const [mobId, visual] of this.mobVisuals) {
      const mob = state.mobs[mobId];
      if (!mob) {
        visual.group.visible = false;
        continue;
      }
      const template = gameTemplates.mobTemplates[mob.templateId];
      visual.group.visible = mob.aiState !== 'dead';
      if (!visual.group.visible) {
        continue;
      }
      visual.group.position.copy(toWorld(mob.position));
      visual.group.rotation.y = Math.sin(state.timeMs * 0.001 + mobId.length) * 0.08;
      const healthRatio = mob.hp / template.maxHp;
      visual.hpBar.scale.x = Math.max(healthRatio, 0.08);
      visual.hpBar.position.x = -(1 - visual.hpBar.scale.x);
    }

    for (const lootId of Array.from(this.lootVisuals.keys())) {
      if (state.loot[lootId]) {
        continue;
      }
      const mesh = this.lootVisuals.get(lootId);
      if (!mesh) {
        continue;
      }
      this.scene.remove(mesh);
      this.clickables.splice(this.clickables.indexOf(mesh), 1);
      this.lootVisuals.delete(lootId);
    }

    for (const [lootId, loot] of Object.entries(state.loot)) {
      let visual = this.lootVisuals.get(lootId);
      if (!visual) {
        visual = createLootVisual();
        visual.userData = { kind: 'loot', id: lootId } satisfies ClickTarget;
        this.lootVisuals.set(lootId, visual);
        this.scene.add(visual);
        this.clickables.push(visual);
      }
      visual.position.copy(toWorld(loot.position));
      visual.position.y = 0.32 + Math.sin(state.timeMs * 0.004 + loot.position.x) * 0.14;
      visual.rotation.y += 0.02;
    }

    for (const [npcId, visual] of this.npcVisuals) {
      const npc = state.npcs[npcId];
      visual.position.copy(toWorld(npc.position));
    }

    const cameraTarget = toWorld(state.player.position, 4);
    const horizontalDistance = Math.cos(INITIAL_CAMERA_ELEVATION) * this.cameraDistance;
    const desiredCamera = cameraTarget.clone().add(
      new THREE.Vector3(
        Math.cos(this.cameraYaw) * horizontalDistance,
        Math.sin(INITIAL_CAMERA_ELEVATION) * this.cameraDistance,
        Math.sin(this.cameraYaw) * horizontalDistance,
      ),
    );
    this.camera.position.lerp(desiredCamera, 0.08);
    this.camera.lookAt(cameraTarget);

    this.updateLabels(state);
  }

  private updatePathLine(line: THREE.Line, points: Vec2[], y: number): void {
    line.visible = points.length >= 2;
    if (!line.visible) {
      return;
    }

    const geometry = new THREE.BufferGeometry().setFromPoints(points.map((point) => toWorld(point, y)));
    line.geometry.dispose();
    line.geometry = geometry;
  }

  private updateLabels(state: GameState): void {
    for (const key of Array.from(this.labelNodes.keys())) {
      if (state.floatingTexts.some((entry) => entry.id === key)) {
        continue;
      }
      const node = this.labelNodes.get(key);
      if (node) {
        node.remove();
      }
      this.labelNodes.delete(key);
    }

    for (const entry of state.floatingTexts) {
      let node = this.labelNodes.get(entry.id);
      if (!node) {
        node = document.createElement('div');
        node.className = 'floating-number';
        this.labelNodes.set(entry.id, node);
        this.labelsHost.appendChild(node);
      }
      node.textContent = entry.text;
      node.style.color = entry.color;

      let worldPoint = toWorld(entry.position, 3.6);
      if (entry.entityId === state.player.id) {
        worldPoint = toWorld(state.player.position, 4.6);
      } else if (entry.entityId && state.otherPlayers[entry.entityId]) {
        worldPoint = toWorld(state.otherPlayers[entry.entityId].position, 4.6);
      } else if (entry.entityId && state.mobs[entry.entityId]) {
        worldPoint = toWorld(state.mobs[entry.entityId].position, 4.3);
      }
      worldPoint.y += (1 - entry.ttlMs / 1100) * 1.8;

      const projected = worldPoint.clone().project(this.camera);
      const x = ((projected.x + 1) / 2) * this.root.clientWidth;
      const y = ((-projected.y + 1) / 2) * this.root.clientHeight;
      node.style.transform = `translate(${x}px, ${y}px)`;
      node.style.opacity = `${Math.min(entry.ttlMs / 250, 1)}`;
    }
  }

  render(): void {
    this.renderer.render(this.scene, this.camera);
  }

  destroy(): void {
    if (this.interactive) {
      this.renderer.domElement.removeEventListener('pointerdown', this.handlePointerDown);
      this.renderer.domElement.removeEventListener('contextmenu', this.handleContextMenu);
      this.renderer.domElement.removeEventListener('wheel', this.handleWheel);
      this.renderer.domElement.removeEventListener('pointermove', this.handleCameraPointerMove);
      this.renderer.domElement.removeEventListener('pointerup', this.handleCameraPointerUp);
      this.renderer.domElement.removeEventListener('pointercancel', this.handleCameraPointerUp);
    }
    window.removeEventListener('resize', this.handleResize);
    this.pendingPathLine.geometry.dispose();
    (this.pendingPathLine.material as THREE.LineBasicMaterial).dispose();
    this.authoritativePathLine.geometry.dispose();
    (this.authoritativePathLine.material as THREE.LineBasicMaterial).dispose();
    for (const visual of this.companionVisuals.values()) {
      this.scene.remove(visual.group);
    }
    this.renderer.dispose();
  }
}
