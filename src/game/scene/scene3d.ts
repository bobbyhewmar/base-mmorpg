import * as THREE from 'three';
import type { Object3D } from 'three';
import { GLTFLoader } from 'three/examples/jsm/loaders/GLTFLoader.js';
import { gameTemplates } from '../data/templates';
import { getEquippedBySlot, getTargetMob, getTemplate, type GameStore } from '../domain/game';
import { ensureClassCharacterModel, updateClassCharacterModelAnimation } from './characterModelAssets';
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
  body: THREE.Mesh;
  head: THREE.Mesh;
  legs: THREE.Mesh[];
  walkPhase: number;
  lastAnimationTimeMs: number | null;
  wasVisible: boolean;
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
  magicAura: THREE.Group;
  walkPhase: number;
  lastAnimationPosition: Vec2 | null;
  lastAnimationTimeMs: number | null;
  basicAttackStartedAtMs: number | null;
  magicCastStartedAtMs: number | null;
  previousBasicAttackCooldownMs: number;
  previousCooldowns: Record<string, number>;
};

type CharacterVisualAppearance = {
  race: CharacterRace;
  baseClass: BaseClass;
  sex: CharacterSex;
  hairStyle: AppearanceOptionIndex;
  hairColor: string;
  skinType: AppearanceOptionIndex;
};

const CAMERA_OFFSET = new THREE.Vector3(-7.5, 9, 7.5);
const INITIAL_CAMERA_DISTANCE = CAMERA_OFFSET.length();
const INITIAL_CAMERA_ELEVATION = Math.asin(CAMERA_OFFSET.y / INITIAL_CAMERA_DISTANCE);
const INITIAL_CAMERA_YAW = Math.atan2(CAMERA_OFFSET.z, CAMERA_OFFSET.x);
const CAMERA_MIN_DISTANCE = 3.8;
const CAMERA_MAX_DISTANCE = 28;
const CAMERA_MIN_ELEVATION = THREE.MathUtils.degToRad(-72);
const CAMERA_MAX_ELEVATION = THREE.MathUtils.degToRad(84);
const CAMERA_GROUND_CLEARANCE = 0.72;
const CAMERA_GROUND_GUARD_MIN_DISTANCE = 2.4;
const CAMERA_TARGET_SMOOTHING_PER_SECOND = 8;
const CAMERA_POSITION_SMOOTHING_PER_SECOND = 7;
const CAMERA_ORBIT_SENSITIVITY = 0.008;
const CAMERA_ELEVATION_SENSITIVITY = 0.006;
const CAMERA_ZOOM_SENSITIVITY = 0.012;
const BASIC_ATTACK_VISUAL_MS = 420;
const MAGIC_CAST_VISUAL_MS = 760;
const STONECROSS_PLAYABLE_SIZE = 1024;
const CHARACTER_WORLD_VISUAL_SCALE = 0.255;
const MOB_WORLD_VISUAL_SCALE = 0.465;
const NPC_WORLD_VISUAL_SCALE = 0.255;
const COMPANION_WORLD_VISUAL_SCALE = 0.405;
const MOUNTED_COMPANION_WORLD_VISUAL_SCALE = 0.495;
const CAMERA_TARGET_HEIGHT = 1.05;
const FLOATING_TEXT_DEFAULT_HEIGHT = 1.95;
const FLOATING_TEXT_CHARACTER_HEIGHT = 1.25;
const FLOATING_TEXT_MOB_HEIGHT = 2.15;
// Tuned for the current reduced player mesh scale so the label stays closer to the head.
const PLAYER_NAMEPLATE_HEAD_OFFSET = 0.06;
const PLAYER_NAMEPLATE_SCREEN_OFFSET_Y = -2;
const TARGET_RING_RADIUS = 0.94;
const DESTINATION_MARKER_RADIUS = 0.55;
const SHOW_PATH_DEBUG_OVERLAY = false;
const MOB_VISUAL_SMOOTHING_PER_SECOND = 9;
const MOB_VISUAL_SNAP_DISTANCE = 10;
const MOB_VISUAL_MOVING_EPSILON = 0.08;

const smoothingAlpha = (speedPerSecond: number, deltaMs: number): number =>
  1 - Math.exp(-speedPerSecond * (deltaMs / 1000));

export const STONECROSS_PLAYABLE_BOUNDS = Object.freeze({
  minX: -STONECROSS_PLAYABLE_SIZE / 2,
  maxX: STONECROSS_PLAYABLE_SIZE / 2,
  minZ: -STONECROSS_PLAYABLE_SIZE / 2,
  maxZ: STONECROSS_PLAYABLE_SIZE / 2,
});

export const clampPointToStonecrossPlayableBounds = (point: Vec2): Vec2 => ({
  x: THREE.MathUtils.clamp(point.x, STONECROSS_PLAYABLE_BOUNDS.minX, STONECROSS_PLAYABLE_BOUNDS.maxX),
  z: THREE.MathUtils.clamp(point.z, STONECROSS_PLAYABLE_BOUNDS.minZ, STONECROSS_PLAYABLE_BOUNDS.maxZ),
});

export const resolveCameraPositionWithGroundGuard = (
  cameraTarget: THREE.Vector3,
  desiredCamera: THREE.Vector3,
): THREE.Vector3 => {
  if (desiredCamera.y >= CAMERA_GROUND_CLEARANCE) {
    return desiredCamera;
  }

  const targetToDesired = desiredCamera.clone().sub(cameraTarget);
  const denominator = desiredCamera.y - cameraTarget.y;
  if (Math.abs(denominator) <= 0.0001) {
    return desiredCamera.clone().setY(CAMERA_GROUND_CLEARANCE);
  }

  const ratioToGroundGuard = THREE.MathUtils.clamp(
    (CAMERA_GROUND_CLEARANCE - cameraTarget.y) / denominator,
    0,
    1,
  );
  const guardedCamera = cameraTarget.clone().add(targetToDesired.multiplyScalar(ratioToGroundGuard));

  const guardedDistance = guardedCamera.distanceTo(cameraTarget);
  if (guardedDistance >= CAMERA_GROUND_GUARD_MIN_DISTANCE) {
    guardedCamera.y = CAMERA_GROUND_CLEARANCE;
    return guardedCamera;
  }

  const horizontalDirection = guardedCamera.clone().sub(cameraTarget);
  horizontalDirection.y = 0;
  if (horizontalDirection.lengthSq() <= 0.0001) {
    horizontalDirection.set(0, 0, 1);
  }
  horizontalDirection.normalize();
  const verticalDelta = Math.max(0, cameraTarget.y - CAMERA_GROUND_CLEARANCE);
  const horizontalDistance = Math.sqrt(
    Math.max(0, CAMERA_GROUND_GUARD_MIN_DISTANCE ** 2 - verticalDelta ** 2),
  );
  return new THREE.Vector3(
    cameraTarget.x + horizontalDirection.x * horizontalDistance,
    CAMERA_GROUND_CLEARANCE,
    cameraTarget.z + horizontalDirection.z * horizontalDistance,
  );
};

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
    return new THREE.Vector3(isFemale ? 0.88 : 0.95, isFemale ? 0.82 : 0.88, isFemale ? 0.88 : 0.95).multiplyScalar(
      CHARACTER_WORLD_VISUAL_SCALE,
    );
  }
  if (appearance.race === 'Orc') {
    return new THREE.Vector3(isFemale ? 1.08 : 1.18, isFemale ? 1.04 : 1.13, isFemale ? 1.08 : 1.18).multiplyScalar(
      CHARACTER_WORLD_VISUAL_SCALE,
    );
  }
  if (isFemale) {
    return new THREE.Vector3(0.9, 0.96, 0.9).multiplyScalar(CHARACTER_WORLD_VISUAL_SCALE);
  }
  return new THREE.Vector3(1, 1, 1).multiplyScalar(CHARACTER_WORLD_VISUAL_SCALE);
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

export const NEUTRAL_NAMEPLATE_COLOR = '#f5f1e8';
export const PVP_NAMEPLATE_COLOR = '#b882ff';
export const PK_NAMEPLATE_COLOR = '#ff5f5f';

type AuthoritativeNameplateState = {
  name: string;
  karma: number;
  pvpFlagged: boolean;
  position: Vec2;
};

export type PlayerNameplateEntry = {
  id: string;
  name: string;
  color: string;
  position: Vec2;
};

export const getPlayerNameplateColor = (state: Pick<AuthoritativeNameplateState, 'karma' | 'pvpFlagged'>): string => {
  if (state.karma > 0) {
    return PK_NAMEPLATE_COLOR;
  }
  if (state.pvpFlagged) {
    return PVP_NAMEPLATE_COLOR;
  }
  return NEUTRAL_NAMEPLATE_COLOR;
};

export const getVisiblePlayerNameplates = (
  state: Pick<GameState, 'player' | 'otherPlayers'>,
): PlayerNameplateEntry[] => [
  {
    id: state.player.id,
    name: state.player.name,
    color: getPlayerNameplateColor(state.player),
    position: state.player.position,
  },
  ...Object.values(state.otherPlayers).map((otherPlayer) => ({
    id: otherPlayer.id,
    name: otherPlayer.name,
    color: getPlayerNameplateColor(otherPlayer),
    position: otherPlayer.position,
  })),
];

const createStoneBox = (width: number, height: number, depth: number, color: string): THREE.Mesh => {
  const mesh = new THREE.Mesh(
    new THREE.BoxGeometry(width, height, depth),
    new THREE.MeshStandardMaterial({ color, roughness: 0.94 }),
  );
  mesh.castShadow = true;
  mesh.receiveShadow = true;
  return mesh;
};

const medievalMapAssetUrls = import.meta.glob('../../assets/maps/medieval-village-megakit/**/*', {
  query: '?url',
  import: 'default',
  eager: true,
}) as Record<string, string>;

const medievalMapAssetUrl = (fileName: string): string => {
  const normalizedFileName = fileName.replace(/\\/g, '/');
  const key = `../../assets/maps/medieval-village-megakit/${normalizedFileName}`;
  const assetUrl = medievalMapAssetUrls[key];
  if (!assetUrl) {
    throw new Error(`Missing canonical Medieval Village MegaKit asset: ${normalizedFileName}`);
  }
  return assetUrl;
};

const retroMapAssetUrl = (fileName: string): string =>
  new URL(`../../assets/maps/retro/${fileName}`, import.meta.url).href;

const MEDIEVAL_MAP_ASSETS = {
  terrainNoise: medievalMapAssetUrl('T_Noise_Terrain.png'),
  vine1: medievalMapAssetUrl('Prop_Vine1.gltf'),
  vine2: medievalMapAssetUrl('Prop_Vine2.gltf'),
  vine4: medievalMapAssetUrl('Prop_Vine4.gltf'),
  vine5: medievalMapAssetUrl('Prop_Vine5.gltf'),
  vine6: medievalMapAssetUrl('Prop_Vine6.gltf'),
  vine9: medievalMapAssetUrl('Prop_Vine9.gltf'),
} as const;

type MedievalMapAssetId = Exclude<keyof typeof MEDIEVAL_MAP_ASSETS, 'terrainNoise'>;

type MedievalPlacement = {
  asset: MedievalMapAssetId;
  position: readonly [number, number, number];
  rotation?: readonly [number, number, number];
  scale?: number | readonly [number, number, number];
  name?: string;
};

const RETRO_MAP_ASSETS = {
  barrels: retroMapAssetUrl('barrels.glb'),
  column: retroMapAssetUrl('column.glb'),
  columnDamaged: retroMapAssetUrl('column-damaged.glb'),
  crate: retroMapAssetUrl('detail-crate.glb'),
  crateRopes: retroMapAssetUrl('detail-crate-ropes.glb'),
  dockSide: retroMapAssetUrl('dock-side.glb'),
  fenceWood: retroMapAssetUrl('fence-wood.glb'),
  pulleyCrate: retroMapAssetUrl('pulley-crate.glb'),
  roof: retroMapAssetUrl('roof.glb'),
  roofHighSide: retroMapAssetUrl('roof-high-side.glb'),
  roofSide: retroMapAssetUrl('roof-side.glb'),
  stairsStone: retroMapAssetUrl('stairs-stone.glb'),
  structure: retroMapAssetUrl('structure.glb'),
  structureWall: retroMapAssetUrl('structure-wall.glb'),
  tower: retroMapAssetUrl('tower.glb'),
  towerTop: retroMapAssetUrl('tower-top.glb'),
  treeLarge: retroMapAssetUrl('tree-large.glb'),
  treeShrub: retroMapAssetUrl('tree-shrub.glb'),
  wallFortified: retroMapAssetUrl('wall-fortified.glb'),
  wallFortifiedGate: retroMapAssetUrl('wall-fortified-gate.glb'),
  wallPanePaintedWood: retroMapAssetUrl('wall-pane-painted-wood.glb'),
  wallPaneWood: retroMapAssetUrl('wall-pane-wood.glb'),
} as const;

type RetroMapAssetId = keyof typeof RETRO_MAP_ASSETS;

type RetroPlacement = {
  asset: RetroMapAssetId;
  position: readonly [number, number, number];
  scale?: number | readonly [number, number, number];
  rotationY?: number;
  name?: string;
};

const mapTextureLoader = new THREE.TextureLoader();
const medievalMapLoader = new GLTFLoader();
const medievalMapTemplateCache = new Map<MedievalMapAssetId, Promise<THREE.Group>>();
const retroMapLoader = new GLTFLoader();
const retroMapTemplateCache = new Map<RetroMapAssetId, Promise<THREE.Group>>();

const isMeshObject = (object: Object3D): object is THREE.Mesh => (object as THREE.Mesh).isMesh === true;

const configureRetroMapObject = (object: Object3D): void => {
  object.traverse((child) => {
    if (!isMeshObject(child)) {
      return;
    }
    child.castShadow = true;
    child.receiveShadow = true;
  });
};

const loadRetroMapTemplate = (asset: RetroMapAssetId): Promise<THREE.Group> => {
  const cached = retroMapTemplateCache.get(asset);
  if (cached) {
    return cached;
  }

  const loading = retroMapLoader.loadAsync(RETRO_MAP_ASSETS[asset]).then((gltf) => {
    const template = gltf.scene;
    configureRetroMapObject(template);
    return template;
  });
  retroMapTemplateCache.set(asset, loading);
  return loading;
};

const loadMedievalMapTemplate = (asset: MedievalMapAssetId): Promise<THREE.Group> => {
  const cached = medievalMapTemplateCache.get(asset);
  if (cached) {
    return cached;
  }

  const loading = medievalMapLoader.loadAsync(MEDIEVAL_MAP_ASSETS[asset]).then((gltf) => {
    const template = gltf.scene;
    configureRetroMapObject(template);
    return template;
  });
  medievalMapTemplateCache.set(asset, loading);
  return loading;
};

const applyRetroScale = (object: Object3D, scale: RetroPlacement['scale']): void => {
  if (scale === undefined) {
    object.scale.set(1, 1, 1);
    return;
  }
  if (typeof scale === 'number') {
    object.scale.setScalar(scale);
    return;
  }
  object.scale.set(scale[0], scale[1], scale[2]);
};

const applyMedievalScale = (object: Object3D, scale: MedievalPlacement['scale']): void => {
  if (scale === undefined) {
    object.scale.set(1, 1, 1);
    return;
  }
  if (typeof scale === 'number') {
    object.scale.setScalar(scale);
    return;
  }
  object.scale.set(scale[0], scale[1], scale[2]);
};

const instantiateRetroMapAsset = async (placement: RetroPlacement): Promise<Object3D> => {
  const template = await loadRetroMapTemplate(placement.asset);
  const object = template.clone(true);
  object.name = placement.name ?? `retro-${placement.asset}`;
  object.position.set(placement.position[0], placement.position[1], placement.position[2]);
  object.rotation.y = placement.rotationY ?? 0;
  applyRetroScale(object, placement.scale);
  configureRetroMapObject(object);
  return object;
};

const instantiateMedievalMapAsset = async (placement: MedievalPlacement): Promise<Object3D> => {
  const template = await loadMedievalMapTemplate(placement.asset);
  const object = template.clone(true);
  object.name = placement.name ?? `medieval-${placement.asset}`;
  object.position.set(placement.position[0], placement.position[1], placement.position[2]);
  if (placement.rotation) {
    object.rotation.set(placement.rotation[0], placement.rotation[1], placement.rotation[2]);
  }
  applyMedievalScale(object, placement.scale);
  configureRetroMapObject(object);
  return object;
};

const createTerrainPlate = (width: number, depth: number, color: string, y = 0): THREE.Mesh => {
  const plate = createStoneBox(width, 0.08, depth, color);
  plate.position.y = y;
  return plate;
};

const rangeInclusive = (start: number, end: number, step: number): number[] => {
  const values: number[] = [];
  for (let value = start; value <= end + 0.001; value += step) {
    values.push(Number(value.toFixed(4)));
  }
  return values;
};

const BUILDING_MODULE_MIN_SCALE = 2.6;
const BUILDING_MODULE_MAX_SCALE = 3.4;
const BUILDING_MODULE_HEIGHT_RATIO = 0.42;
const CITY_WALL_SCALE = 3.2;
const CITY_WALL_STEP = CITY_WALL_SCALE * 0.96;
const CITY_GATE_SCALE = 4.2;
const CITY_TOWER_SCALE = 5.2;
const CITY_TOWER_TOP_SCALE = 5;
const COLUMN_SCALE = 3.2;
const STATUE_SCALE = 4.2;
const STAIR_SCALE = 3.6;
const DOCK_SCALE = 4;
const FENCE_SCALE = 3.8;
const CLEAN_REGION_TERRAIN_TEXTURE_REPEAT = 5333;
const CLEAN_REGION_GROUND_DECORATION_BASE_SCALE = 1.8;
const CLEAN_REGION_GROUND_DECORATION_SCALE_STEP = 0.28;

const createRetroBuildingPlacements = (
  x: number,
  z: number,
  options: {
    width: number;
    depth: number;
    height: number;
    body?: RetroMapAssetId;
    roof?: RetroMapAssetId;
    rotationY?: number;
  },
): RetroPlacement[] => {
  const body = options.body ?? 'structure';
  const roof = options.roof ?? 'roof';
  const rotationY = options.rotationY ?? 0;
  const wallScale = Math.max(
    BUILDING_MODULE_MIN_SCALE,
    Math.min(options.height * BUILDING_MODULE_HEIGHT_RATIO, BUILDING_MODULE_MAX_SCALE),
  );
  const roofScale = wallScale;
  const wallStep = wallScale * 0.94;
  const roofStep = roofScale * 0.94;
  const frontCount = Math.max(2, Math.round(options.width / wallStep));
  const sideCount = Math.max(2, Math.round(options.depth / wallStep));
  const actualWidth = (frontCount - 1) * wallStep;
  const actualDepth = (sideCount - 1) * wallStep;
  const roofXCount = Math.max(1, Math.ceil((actualWidth + roofStep) / roofStep));
  const roofZCount = Math.max(1, Math.ceil((actualDepth + roofStep) / roofStep));
  const rotatedPosition = (offsetX: number, y: number, offsetZ: number): readonly [number, number, number] => {
    const cos = Math.cos(rotationY);
    const sin = Math.sin(rotationY);
    return [x + offsetX * cos - offsetZ * sin, y, z + offsetX * sin + offsetZ * cos] as const;
  };

  const placements: RetroPlacement[] = [];
  const pushWall = (offsetX: number, offsetZ: number, sideRotationY: number, name: string): void => {
    placements.push({
      asset: body,
      position: rotatedPosition(offsetX, 0, offsetZ),
      scale: wallScale,
      rotationY: rotationY + sideRotationY,
      name,
    });
  };

  for (let index = 0; index < frontCount; index += 1) {
    const offsetX = -actualWidth / 2 + index * wallStep;
    pushWall(offsetX, -actualDepth / 2, 0, `stonecross-building-front-${x}-${z}-${index}`);
    pushWall(offsetX, actualDepth / 2, Math.PI, `stonecross-building-back-${x}-${z}-${index}`);
  }

  for (let index = 1; index < sideCount - 1; index += 1) {
    const offsetZ = -actualDepth / 2 + index * wallStep;
    pushWall(-actualWidth / 2, offsetZ, Math.PI / 2, `stonecross-building-left-${x}-${z}-${index}`);
    pushWall(actualWidth / 2, offsetZ, -Math.PI / 2, `stonecross-building-right-${x}-${z}-${index}`);
  }

  for (let ix = 0; ix < roofXCount; ix += 1) {
    const offsetX = -(roofXCount - 1) * roofStep * 0.5 + ix * roofStep;
    for (let iz = 0; iz < roofZCount; iz += 1) {
      const offsetZ = -(roofZCount - 1) * roofStep * 0.5 + iz * roofStep;
      placements.push({
        asset: roof,
        position: rotatedPosition(offsetX, wallScale - 0.05, offsetZ),
        scale: roofScale,
        rotationY,
        name: `stonecross-building-roof-${x}-${z}-${ix}-${iz}`,
      });
    }
  }

  return placements;
};

const createStonecrossTerrainBase = (scene: THREE.Scene): void => {
  const terrainTexture = mapTextureLoader.load(MEDIEVAL_MAP_ASSETS.terrainNoise);
  terrainTexture.colorSpace = THREE.SRGBColorSpace;
  terrainTexture.wrapS = THREE.RepeatWrapping;
  terrainTexture.wrapT = THREE.RepeatWrapping;
  terrainTexture.repeat.set(CLEAN_REGION_TERRAIN_TEXTURE_REPEAT, CLEAN_REGION_TERRAIN_TEXTURE_REPEAT);
  terrainTexture.needsUpdate = true;

  const base = new THREE.Mesh(
    new THREE.PlaneGeometry(STONECROSS_PLAYABLE_SIZE, STONECROSS_PLAYABLE_SIZE, 1, 1),
    new THREE.MeshStandardMaterial({
      map: terrainTexture,
      color: '#5f7145',
      roughness: 0.98,
      metalness: 0,
    }),
  );
  base.rotation.x = -Math.PI / 2;
  base.position.set(0, 0, 0);
  base.name = 'clean-region-ground-1024';
  base.receiveShadow = true;
  scene.add(base);
};

const createStonecrossMedievalGroundPlacements = (): MedievalPlacement[] => {
  const vineAssets: MedievalMapAssetId[] = ['vine1', 'vine2', 'vine4', 'vine5', 'vine6', 'vine9'];
  const placements: MedievalPlacement[] = [];
  let index = 0;

  for (const x of rangeInclusive(-456, 456, 76)) {
    for (const z of rangeInclusive(-456, 456, 76)) {
      if ((index + Math.round(x) + Math.round(z)) % 3 !== 0) {
        index += 1;
        continue;
      }
      const offsetX = ((index * 37) % 31) - 15;
      const offsetZ = ((index * 53) % 29) - 14;
      placements.push({
        asset: vineAssets[index % vineAssets.length],
        position: [x + offsetX, 0.018, z + offsetZ],
        rotation: [-Math.PI / 2, (index * 0.73) % (Math.PI * 2), 0],
        scale: CLEAN_REGION_GROUND_DECORATION_BASE_SCALE + (index % 5) * CLEAN_REGION_GROUND_DECORATION_SCALE_STEP,
        name: `clean-region-ground-vine-${index}`,
      });
      index += 1;
    }
  }

  return placements;
};

const createStonecrossRetroPlacements = (): RetroPlacement[] => {
  const placements: RetroPlacement[] = [
    ...createRetroBuildingPlacements(-76, 24, {
      width: 18,
      depth: 14,
      height: 6.5,
      body: 'wallPaneWood',
      roof: 'roofSide',
    }),
    ...createRetroBuildingPlacements(-74, -24, {
      width: 20,
      depth: 15,
      height: 6.8,
      body: 'wallPanePaintedWood',
      roof: 'roof',
    }),
    ...createRetroBuildingPlacements(-42, -42, {
      width: 21,
      depth: 15,
      height: 7.2,
      body: 'wallPanePaintedWood',
      roof: 'roofHighSide',
    }),
    ...createRetroBuildingPlacements(-34, 42, {
      width: 22,
      depth: 15,
      height: 6.7,
      body: 'wallPaneWood',
      roof: 'roofSide',
    }),
    ...createRetroBuildingPlacements(24, -58, {
      width: 24,
      depth: 18,
      height: 7.4,
      body: 'wallPanePaintedWood',
      roof: 'roof',
    }),
    ...createRetroBuildingPlacements(38, -52, {
      width: 18,
      depth: 14,
      height: 7,
      body: 'wallPaneWood',
      roof: 'roofHighSide',
    }),
    ...createRetroBuildingPlacements(70, 26, {
      width: 30,
      depth: 24,
      height: 8.5,
      body: 'wallPanePaintedWood',
      roof: 'roofHighSide',
    }),
    ...createRetroBuildingPlacements(64, 104, {
      width: 26,
      depth: 20,
      height: 7.5,
      body: 'wallPaneWood',
      roof: 'roof',
    }),
    ...createRetroBuildingPlacements(-102, -18, {
      width: 18,
      depth: 14,
      height: 6.5,
      body: 'wallPaneWood',
      roof: 'roofSide',
      rotationY: Math.PI / 2,
    }),
  ];

  for (const x of rangeInclusive(-128, 112, CITY_WALL_STEP)) {
    if (Math.abs(x + 8) > 18) {
      placements.push(
        { asset: 'wallFortified', position: [x, 0, -92], scale: CITY_WALL_SCALE },
        { asset: 'wallFortified', position: [x, 0, 92], scale: CITY_WALL_SCALE },
      );
    }
  }

  for (const z of rangeInclusive(-84, 84, CITY_WALL_STEP)) {
    if (Math.abs(z) > 18) {
      placements.push(
        { asset: 'wallFortified', position: [-136, 0, z], scale: CITY_WALL_SCALE, rotationY: Math.PI / 2 },
        { asset: 'wallFortified', position: [120, 0, z], scale: CITY_WALL_SCALE, rotationY: Math.PI / 2 },
      );
    }
  }

  placements.push(
    { asset: 'wallFortifiedGate', position: [-12, 0, -92], scale: CITY_GATE_SCALE },
    { asset: 'wallFortifiedGate', position: [-8, 0, -92], scale: CITY_GATE_SCALE },
    { asset: 'wallFortifiedGate', position: [-4, 0, -92], scale: CITY_GATE_SCALE },
    { asset: 'wallFortifiedGate', position: [-12, 0, 92], scale: CITY_GATE_SCALE },
    { asset: 'wallFortifiedGate', position: [-8, 0, 92], scale: CITY_GATE_SCALE },
    { asset: 'wallFortifiedGate', position: [-4, 0, 92], scale: CITY_GATE_SCALE },
    { asset: 'wallFortifiedGate', position: [-136, 0, -4], scale: CITY_GATE_SCALE, rotationY: Math.PI / 2 },
    { asset: 'wallFortifiedGate', position: [-136, 0, 0], scale: CITY_GATE_SCALE, rotationY: Math.PI / 2 },
    { asset: 'wallFortifiedGate', position: [-136, 0, 4], scale: CITY_GATE_SCALE, rotationY: Math.PI / 2 },
    { asset: 'wallFortifiedGate', position: [120, 0, -4], scale: CITY_GATE_SCALE, rotationY: Math.PI / 2 },
    { asset: 'wallFortifiedGate', position: [120, 0, 0], scale: CITY_GATE_SCALE, rotationY: Math.PI / 2 },
    { asset: 'wallFortifiedGate', position: [120, 0, 4], scale: CITY_GATE_SCALE, rotationY: Math.PI / 2 },
  );

  for (const [x, z] of [
    [-136, -92],
    [120, -92],
    [-136, 92],
    [120, 92],
  ] as const) {
    placements.push(
      { asset: 'tower', position: [x, 0, z], scale: CITY_TOWER_SCALE },
      { asset: 'towerTop', position: [x, CITY_TOWER_SCALE, z], scale: CITY_TOWER_TOP_SCALE },
    );
  }

  for (const [x, z] of [
    [48, 7],
    [92, 7],
    [48, 45],
    [92, 45],
    [-20, -10],
    [-20, 10],
    [4, 10],
    [4, -10],
  ] as const) {
    placements.push({ asset: 'column', position: [x, 0, z], scale: COLUMN_SCALE });
  }

  placements.push(
    { asset: 'columnDamaged', position: [-6, 0, 0], scale: STATUE_SCALE, name: 'stonecross-central-statue' },
    { asset: 'stairsStone', position: [70, 0, 2], scale: STAIR_SCALE, rotationY: Math.PI },
    { asset: 'stairsStone', position: [64, 0, 78], scale: STAIR_SCALE },
    { asset: 'dockSide', position: [112, 0, -18], scale: DOCK_SCALE },
    { asset: 'dockSide', position: [38, 0, 106], scale: DOCK_SCALE, rotationY: Math.PI / 2 },
    { asset: 'pulleyCrate', position: [8, 0, -8], scale: 2.5 },
    { asset: 'barrels', position: [-23, 0, -4], scale: 2.6 },
    { asset: 'crate', position: [-18, 0, 4], scale: 2.4 },
    { asset: 'crateRopes', position: [-28, 0, 7], scale: 2.5 },
    { asset: 'barrels', position: [-86, 0, 46], scale: 2.6 },
    { asset: 'crate', position: [28, 0, -15], scale: 2.4 },
  );

  for (const x of rangeInclusive(-460, 460, 92)) {
    if (Math.abs(x + 8) > 38) {
      placements.push(
        { asset: 'treeLarge', position: [x, 0, -430], scale: 5 },
        { asset: 'treeLarge', position: [x + 22, 0, 430], scale: 4.8, rotationY: Math.PI * 0.4 },
        { asset: 'treeShrub', position: [x - 18, 0, -350], scale: 3.5 },
        { asset: 'treeShrub', position: [x + 18, 0, 350], scale: 3.5 },
      );
    }
  }

  for (const z of rangeInclusive(-360, 360, 92)) {
    if (Math.abs(z) > 38) {
      placements.push(
        { asset: 'treeLarge', position: [-448, 0, z], scale: 5, rotationY: Math.PI * 0.2 },
        { asset: 'treeLarge', position: [448, 0, z + 24], scale: 4.8, rotationY: Math.PI * 0.7 },
        { asset: 'treeShrub', position: [-360, 0, z - 22], scale: 3.4 },
        { asset: 'treeShrub', position: [360, 0, z + 18], scale: 3.4 },
      );
    }
  }

  for (const [x, z, rotationY] of [
    [-220, -18, 0],
    [-250, 24, 0],
    [210, -28, 0],
    [238, 28, 0],
    [-34, -212, Math.PI / 2],
    [20, -236, Math.PI / 2],
    [-30, 210, Math.PI / 2],
    [32, 238, Math.PI / 2],
  ] as const) {
    placements.push({ asset: 'fenceWood', position: [x, 0, z], scale: FENCE_SCALE, rotationY });
  }

  return placements;
};

const loadStonecrossRetroMap = async (root: THREE.Group): Promise<void> => {
  const placements = createStonecrossRetroPlacements();
  const objects = await Promise.all(placements.map((placement) => instantiateRetroMapAsset(placement)));
  for (const object of objects) {
    root.add(object);
  }
};

const createStonecrossPlazaWorld = (scene: THREE.Scene): void => {
  createStonecrossTerrainBase(scene);
  const groundDecorationRoot = new THREE.Group();
  groundDecorationRoot.name = 'clean-region-ground-decoration-megakit';
  scene.add(groundDecorationRoot);
  void Promise.all(createStonecrossMedievalGroundPlacements().map((placement) => instantiateMedievalMapAsset(placement)))
    .then((objects) => {
      for (const object of objects) {
        groundDecorationRoot.add(object);
      }
    })
    .catch((error: unknown) => {
      console.error('Failed to load Medieval Village MegaKit ground decoration.', error);
    });
};

const createMobVisual = (tint: string, visualScale = MOB_WORLD_VISUAL_SCALE): MobVisual => {
  const group = new THREE.Group();
  group.scale.setScalar(visualScale);

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
  const legs: THREE.Mesh[] = [];
  for (const [x, z] of [
    [-0.55, -0.4],
    [-0.55, 0.4],
    [0.35, -0.4],
    [0.35, 0.4],
  ]) {
    const leg = new THREE.Mesh(legGeometry, legMaterial);
    leg.position.set(x, 0.7, z);
    group.add(leg);
    legs.push(leg);
  }

  const hpBar = new THREE.Mesh(
    new THREE.BoxGeometry(2, 0.16, 0.12),
    new THREE.MeshBasicMaterial({ color: '#d85c60' }),
  );
  hpBar.position.set(0, 3.8, 0);

  group.add(body, head, hornLeft, hornRight, hpBar);
  return {
    group,
    hpBar,
    body,
    head,
    legs,
    walkPhase: 0,
    lastAnimationTimeMs: null,
    wasVisible: false,
  };
};

const mobInterpolationAlpha = (deltaMs: number): number =>
  smoothingAlpha(MOB_VISUAL_SMOOTHING_PER_SECOND, deltaMs);

const faceMobTowardMovement = (visual: MobVisual, from: THREE.Vector3, to: THREE.Vector3): void => {
  const deltaX = to.x - from.x;
  const deltaZ = to.z - from.z;
  if (Math.hypot(deltaX, deltaZ) <= MOB_VISUAL_MOVING_EPSILON) {
    return;
  }
  visual.group.rotation.y = -Math.atan2(deltaZ, deltaX);
};

const animateMobProcedural = (
  visual: MobVisual,
  timeMs: number,
  deltaMs: number,
  moving: boolean,
  aggressive: boolean,
): void => {
  if (moving) {
    visual.walkPhase += deltaMs * (aggressive ? 0.0045 : 0.0035);
  } else {
    visual.walkPhase += deltaMs * 0.0008;
  }

  const stride = Math.sin(visual.walkPhase);
  const counterStride = Math.sin(visual.walkPhase + Math.PI);
  const bob = moving ? Math.abs(stride) * 0.035 : Math.sin(timeMs * 0.001 + visual.group.id) * 0.012;
  const lean = moving ? THREE.MathUtils.clamp(stride * 0.025, -0.025, 0.025) : Math.sin(timeMs * 0.0009) * 0.01;

  visual.body.position.y = 1.7 + bob;
  visual.body.rotation.z = lean;
  visual.body.rotation.x = moving ? -Math.abs(stride) * 0.012 : 0;

  visual.head.position.y = 2.4 + bob * 0.45;
  visual.head.rotation.z = lean * 0.7;
  visual.head.rotation.x = moving ? -0.035 - Math.abs(stride) * 0.015 : Math.sin(timeMs * 0.001) * 0.012;

  for (const [index, leg] of visual.legs.entries()) {
    const phase = index % 2 === 0 ? stride : counterStride;
    leg.rotation.x = moving ? phase * 0.22 : Math.sin(timeMs * 0.0008 + index) * 0.018;
    leg.rotation.z = moving ? phase * 0.04 : 0;
    leg.position.y = 0.7 + (moving ? Math.max(0, phase) * 0.025 : 0);
  }
};

const updateMobVisualMotion = (
  visual: MobVisual,
  targetPosition: THREE.Vector3,
  timeMs: number,
  options: { visible: boolean; aggressive?: boolean },
): void => {
  if (!options.visible) {
    visual.wasVisible = false;
    visual.lastAnimationTimeMs = null;
    return;
  }

  const previousTimeMs = visual.lastAnimationTimeMs;
  const deltaMs = previousTimeMs === null ? 16 : Math.max(0, Math.min(timeMs - previousTimeMs, 80));
  const currentPosition = visual.group.position.clone();
  const distanceToTarget = currentPosition.distanceTo(targetPosition);
  const shouldSnap = !visual.wasVisible || previousTimeMs === null || distanceToTarget > MOB_VISUAL_SNAP_DISTANCE;
  const moving = distanceToTarget > MOB_VISUAL_MOVING_EPSILON;

  if (shouldSnap) {
    visual.group.position.copy(targetPosition);
  } else {
    faceMobTowardMovement(visual, currentPosition, targetPosition);
    visual.group.position.lerp(targetPosition, mobInterpolationAlpha(deltaMs));
  }

  animateMobProcedural(visual, timeMs, deltaMs, moving, options.aggressive === true);
  visual.lastAnimationTimeMs = timeMs;
  visual.wasVisible = true;
};

const createNpcVisual = (): THREE.Group => {
  const group = new THREE.Group();
  group.scale.setScalar(NPC_WORLD_VISUAL_SCALE);
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
    new THREE.OctahedronGeometry(0.22, 0),
    new THREE.MeshStandardMaterial({ color: '#f0c774', emissive: '#5a410f', roughness: 0.45 }),
  );
  stone.position.y = 0.17;

  const hitArea = new THREE.Mesh(
    new THREE.CylinderGeometry(0.58, 0.58, 0.16, 12),
    new THREE.MeshBasicMaterial({
      transparent: true,
      opacity: 0,
      depthWrite: false,
    }),
  );
  hitArea.position.y = 0.08;
  group.add(stone, hitArea);
  return group;
};

const createPlayerVisual = (): PlayerVisual => {
  const group = new THREE.Group();
  const proceduralRoot = new THREE.Group();
  proceduralRoot.visible = false;

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

  const magicAura = new THREE.Group();
  const magicRing = new THREE.Mesh(
    new THREE.TorusGeometry(0.72, 0.025, 8, 48),
    new THREE.MeshBasicMaterial({ color: '#7ecbff', transparent: true, opacity: 0.78 }),
  );
  magicRing.rotation.x = Math.PI * 0.5;
  magicRing.position.y = 2.55;
  const magicCore = new THREE.Mesh(
    new THREE.OctahedronGeometry(0.18, 0),
    new THREE.MeshBasicMaterial({ color: '#d9f4ff', transparent: true, opacity: 0.9 }),
  );
  magicCore.position.set(0.8, 2.65, 0.18);
  magicAura.add(magicRing, magicCore);
  magicAura.visible = false;

  proceduralRoot.add(torso, hip, head, hair, faceMark, armLeft, armRight, legLeft, legRight, mantle, weapon, magicAura);
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
    magicAura,
    walkPhase: 0,
    lastAnimationPosition: null,
    lastAnimationTimeMs: null,
    basicAttackStartedAtMs: null,
    magicCastStartedAtMs: null,
    previousBasicAttackCooldownMs: 0,
    previousCooldowns: {},
  };
};

const animatePlayerVisual = (
  visual: PlayerVisual,
  timeMs: number,
  position: Vec2,
  options: {
    movingHint?: boolean;
    movementMode?: 'run' | 'walk';
    casting?: boolean;
    dead?: boolean;
    basicAttackCooldownMs?: number;
    cooldowns?: Record<string, number>;
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

  if (options.cooldowns) {
    for (const [skillId, remainingMs] of Object.entries(options.cooldowns)) {
      if (skillId === 'basic_attack') {
        continue;
      }
      const previousRemainingMs = visual.previousCooldowns[skillId] ?? 0;
      if (remainingMs > previousRemainingMs + 160) {
        visual.magicCastStartedAtMs = timeMs;
      }
    }
    visual.previousCooldowns = { ...options.cooldowns };
  }

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
  visual.magicAura.visible = false;

  if (options.dead) {
    visual.proceduralRoot.rotation.z = Math.PI * 0.5;
    visual.proceduralRoot.position.y = 0.72;
    visual.group.position.y = 0;
    visual.magicAura.visible = false;
    updateClassCharacterModelAnimation(visual.group, deltaMs, { dead: true });
    return;
  }
  visual.proceduralRoot.position.y = 0;

  const idle = Math.sin(timeMs * 0.0021 + visual.group.id * 0.17);
  const walkSwing = Math.sin(visual.walkPhase);
  const walkCounterSwing = Math.cos(visual.walkPhase * 2);
  const walkAmount = moving ? 1 : 0;
  const idleAmount = moving ? 0 : 1;

  visual.group.position.y = moving ? 0 : idle * 0.006;
  visual.torso.rotation.z = walkCounterSwing * 0.026 * walkAmount + idle * 0.012 * idleAmount;
  visual.hip.rotation.z = -visual.torso.rotation.z * 0.55;
  visual.head.position.y = 3.65 + idle * 0.008 * idleAmount;
  visual.hair.position.y = 3.78 + idle * 0.007 * idleAmount;
  visual.faceMark.position.y = 3.52 + idle * 0.006 * idleAmount;

  visual.legLeft.rotation.x = -walkSwing * 0.58 * walkAmount;
  visual.legRight.rotation.x = walkSwing * 0.58 * walkAmount;
  visual.armLeft.rotation.x = walkSwing * 0.62 * walkAmount;
  visual.armRight.rotation.x = -walkSwing * 0.62 * walkAmount;

  const magicProgress =
    visual.magicCastStartedAtMs === null ? null : (timeMs - visual.magicCastStartedAtMs) / MAGIC_CAST_VISUAL_MS;
  const magicActive = options.casting === true || (magicProgress !== null && magicProgress < 1);
  if (magicProgress !== null && magicProgress >= 1) {
    visual.magicCastStartedAtMs = null;
  }

  if (magicActive) {
    const resolvedProgress = options.casting ? 0.45 : Math.max(0, Math.min(magicProgress ?? 0, 1));
    const pulse = Math.sin(timeMs * 0.011) * 0.08;
    const burst = Math.sin(resolvedProgress * Math.PI);
    visual.torso.rotation.x = pulse * 0.28;
    visual.armLeft.rotation.x = -0.78 + pulse - burst * 0.24;
    visual.armRight.rotation.x = -0.78 - pulse - burst * 0.24;
    visual.armLeft.rotation.z = 0.42 + burst * 0.18;
    visual.armRight.rotation.z = -0.42 - burst * 0.18;
    visual.weapon.rotation.x = -0.32 + pulse - burst * 0.18;
    visual.magicAura.visible = true;
    visual.magicAura.rotation.y = timeMs * 0.006;
    visual.magicAura.scale.setScalar(0.78 + burst * 0.55);
    visual.magicAura.position.y = burst * 0.08;
    updateClassCharacterModelAnimation(visual.group, deltaMs, { casting: true });
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
    updateClassCharacterModelAnimation(visual.group, deltaMs, { basicAttacking: true });
    return;
  }
  updateClassCharacterModelAnimation(visual.group, deltaMs, { moving, movementMode: options.movementMode });
};

const applyPlayerVisualAppearance = (
  visual: PlayerVisual,
  appearance: CharacterVisualAppearance,
  options: { dead?: boolean } = {},
): void => {
  const hairStyle = requireAppearanceIndex(appearance.hairStyle, 'hairStyle');
  const skinType = requireAppearanceIndex(appearance.skinType, 'skinType');
  const scale = characterScaleFor(appearance);
  visual.group.scale.copy(scale);
  ensureClassCharacterModel(visual.group, appearance, {
    desiredHeight: 3.72,
    proceduralRoot: visual.proceduralRoot,
  });
  for (const mesh of visual.skinMeshes) {
    setMeshColor(mesh, SKIN_COLORS[appearance.race][skinType]);
  }
  setMeshColor(visual.hair, appearance.hairColor);
  setMeshColor(visual.torso, options.dead ? '#63505a' : gearColorFor(appearance));
  setMeshColor(visual.hip, options.dead ? '#4c4348' : gearColorFor(appearance));
  for (const mesh of visual.legMeshes) {
    setMeshColor(mesh, options.dead ? '#3c3438' : legColorFor(appearance));
  }
  visual.hair.scale.set(1 + hairStyle * 0.08, 0.92 + hairStyle * 0.08, 1 + hairStyle * 0.04);
  visual.hair.position.y = 3.78 - hairStyle * 0.035;
  visual.faceMark.scale.set(0.72, 1, 1);
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
  private readonly targetRing = createCircle(TARGET_RING_RADIUS, '#ffd76e', 0.95);
  private readonly destinationMarker = createCircle(DESTINATION_MARKER_RADIUS, '#8dd9ff', 0.85);
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
  private cameraElevation = THREE.MathUtils.clamp(INITIAL_CAMERA_ELEVATION, CAMERA_MIN_ELEVATION, CAMERA_MAX_ELEVATION);
  private cameraDistance = INITIAL_CAMERA_DISTANCE;
  private movementVisualMode: 'run' | 'walk' = 'run';
  private readonly smoothedCameraTarget = new THREE.Vector3();
  private hasSmoothedCameraTarget = false;
  private lastCameraUpdateTimeMs: number | null = null;
  private isCameraOrbiting = false;
  private lastOrbitPointerX = 0;
  private lastOrbitPointerY = 0;

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
    sun.shadow.camera.far = 170;
    sun.shadow.camera.left = -96;
    sun.shadow.camera.right = 96;
    sun.shadow.camera.top = 96;
    sun.shadow.camera.bottom = -96;
    this.scene.add(ambient, sun);

    createStonecrossPlazaWorld(this.scene);

    this.groundMesh = new THREE.Mesh(
      new THREE.PlaneGeometry(STONECROSS_PLAYABLE_SIZE, STONECROSS_PLAYABLE_SIZE),
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

  setMovementVisualMode(mode: 'run' | 'walk'): void {
    this.movementVisualMode = mode;
  }

  getCameraYaw(): number {
    return this.cameraYaw;
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
    const visual = createMobVisual(
      template.tint,
      companion.mounted ? MOUNTED_COMPANION_WORLD_VISUAL_SCALE : COMPANION_WORLD_VISUAL_SCALE,
    );
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
    visual.group.userData = { kind: 'player', id: otherPlayer.id } satisfies ClickTarget;
    visual.weapon.visible = false;
    visual.mantle.visible = false;
    visual.group.castShadow = true;
    visual.group.receiveShadow = true;
    this.otherPlayerVisuals.set(otherPlayer.id, visual);
    this.scene.add(visual.group);
    this.clickables.push(visual.group);
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
    this.lastOrbitPointerY = event.clientY;
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
    const deltaY = event.clientY - this.lastOrbitPointerY;
    this.lastOrbitPointerX = event.clientX;
    this.lastOrbitPointerY = event.clientY;
    this.cameraYaw += deltaX * CAMERA_ORBIT_SENSITIVITY;
    this.cameraElevation = THREE.MathUtils.clamp(
      this.cameraElevation + deltaY * CAMERA_ELEVATION_SENSITIVITY,
      CAMERA_MIN_ELEVATION,
      CAMERA_MAX_ELEVATION,
    );
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
    const clickable = selectClickTarget(clickableHits, {
      isTargetActive: (target) => this.isClickTargetActive(target),
    });
    if (clickable) {
      if (clickable.kind === 'mob') {
        if (this.onSelectTarget) {
          this.onSelectTarget(clickable.id);
        } else {
          this.store.dispatch({ type: 'selectTarget', targetId: clickable.id });
        }
      } else if (clickable.kind === 'player') {
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
    const clampedPoint = clampPointToStonecrossPlayableBounds(point);
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

  private isClickTargetActive(target: ClickTarget): boolean {
    const state = this.store.getState();
    if (target.kind === 'mob') {
      const mob = state.mobs[target.id];
      return Boolean(mob && mob.aiState !== 'dead');
    }
    if (target.kind === 'loot') {
      return Boolean(state.loot[target.id]);
    }
    if (target.kind === 'npc') {
      return Boolean(state.npcs[target.id]);
    }
    return false;
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
      cooldowns: state.player.cooldowns,
      movementMode: state.player.movementMode ?? this.movementVisualMode,
    });

    const gear = getEquippedBySlot(state);
    this.player.weapon.visible = Boolean(gear.weapon);
    this.player.mantle.visible = Boolean(gear.chest);
    if (gear.weapon) {
      const template = getTemplate(gear.weapon.templateId);
      const tint = template.appearance?.tint ?? '#cbb98d';
      this.player.weapon.scale.setScalar(template.appearance?.weaponModel === 'staff' ? 0.92 : 1);
      for (const child of this.player.weapon.children) {
        const mesh = child as THREE.Mesh;
        if (mesh.material instanceof THREE.Material && hasMaterialColor(mesh.material)) {
          mesh.material.color.set(tint);
        }
      }
    }
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
      animatePlayerVisual(visual, state.timeMs, otherPlayer.position, {
        dead: otherPlayer.dead,
        movementMode: otherPlayer.movementMode,
      });
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
      visual.group.scale.setScalar(
        companion.mounted ? MOUNTED_COMPANION_WORLD_VISUAL_SCALE : COMPANION_WORLD_VISUAL_SCALE,
      );
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
      const isVisible = mob.aiState !== 'dead';
      visual.group.visible = isVisible;
      updateMobVisualMotion(visual, toWorld(mob.position), state.timeMs, {
        visible: isVisible,
        aggressive: mob.aiState === 'aggro',
      });
      if (!isVisible) {
        continue;
      }
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
      visual.position.y = 0.04 + Math.sin(state.timeMs * 0.004 + loot.position.x) * 0.025;
      visual.rotation.y += 0.02;
    }

    for (const [npcId, visual] of this.npcVisuals) {
      const npc = state.npcs[npcId];
      visual.position.copy(toWorld(npc.position));
    }

    const cameraDeltaMs =
      this.lastCameraUpdateTimeMs === null ? 16 : Math.max(0, Math.min(state.timeMs - this.lastCameraUpdateTimeMs, 80));
    this.lastCameraUpdateTimeMs = state.timeMs;

    const desiredCameraTarget = toWorld(state.player.position, CAMERA_TARGET_HEIGHT);
    if (!this.hasSmoothedCameraTarget) {
      this.smoothedCameraTarget.copy(desiredCameraTarget);
      this.hasSmoothedCameraTarget = true;
    } else {
      this.smoothedCameraTarget.lerp(
        desiredCameraTarget,
        smoothingAlpha(CAMERA_TARGET_SMOOTHING_PER_SECOND, cameraDeltaMs),
      );
    }

    const cameraTarget = this.smoothedCameraTarget;
    const horizontalDistance = Math.cos(this.cameraElevation) * this.cameraDistance;
    const desiredCamera = cameraTarget.clone().add(
      new THREE.Vector3(
        Math.cos(this.cameraYaw) * horizontalDistance,
        Math.sin(this.cameraElevation) * this.cameraDistance,
        Math.sin(this.cameraYaw) * horizontalDistance,
      ),
    );
    const guardedCamera = resolveCameraPositionWithGroundGuard(cameraTarget, desiredCamera);
    this.camera.position.lerp(guardedCamera, smoothingAlpha(CAMERA_POSITION_SMOOTHING_PER_SECOND, cameraDeltaMs));
    this.camera.lookAt(cameraTarget);

    this.updateLabels(state);
  }

  private updatePathLine(line: THREE.Line, points: Vec2[], y: number): void {
    if (!SHOW_PATH_DEBUG_OVERLAY) {
      line.visible = false;
      return;
    }

    line.visible = points.length >= 2;
    if (!line.visible) {
      return;
    }

    const geometry = new THREE.BufferGeometry().setFromPoints(points.map((point) => toWorld(point, y)));
    line.geometry.dispose();
    line.geometry = geometry;
  }

  private updateLabels(state: GameState): void {
    const labelEntries = new Map<
      string,
      {
        text: string;
        color: string;
        className: string;
        worldPoint: THREE.Vector3;
        opacity: number;
        anchorTransform: string;
      }
    >();

    const resolvePlayerLabelWorldPoint = (id: string, fallbackPosition: Vec2): THREE.Vector3 => {
      const visual = id === state.player.id ? this.player : this.otherPlayerVisuals.get(id);
      if (!visual) {
        return toWorld(fallbackPosition, FLOATING_TEXT_CHARACTER_HEIGHT);
      }
      const headCenter = visual.head.getWorldPosition(new THREE.Vector3());
      const headRadius = visual.head.geometry.boundingSphere?.radius ?? 0.45;
      const headTopOffset = headRadius * visual.group.scale.y + PLAYER_NAMEPLATE_HEAD_OFFSET;
      const worldPoint = headCenter;
      worldPoint.y += headTopOffset;
      return worldPoint;
    };

    for (const nameplate of getVisiblePlayerNameplates(state)) {
      labelEntries.set(`nameplate:${nameplate.id}`, {
        text: nameplate.name,
        color: nameplate.color,
        className: 'character-nameplate',
        worldPoint: resolvePlayerLabelWorldPoint(nameplate.id, nameplate.position),
        opacity: 1,
        anchorTransform: 'translate(-50%, -100%)',
      });
    }

    for (const entry of state.floatingTexts) {
      let worldPoint = toWorld(entry.position, FLOATING_TEXT_DEFAULT_HEIGHT);
      if (entry.entityId === state.player.id) {
        worldPoint = toWorld(state.player.position, FLOATING_TEXT_CHARACTER_HEIGHT);
      } else if (entry.entityId && state.otherPlayers[entry.entityId]) {
        worldPoint = toWorld(state.otherPlayers[entry.entityId].position, FLOATING_TEXT_CHARACTER_HEIGHT);
      } else if (entry.entityId && state.mobs[entry.entityId]) {
        worldPoint = toWorld(state.mobs[entry.entityId].position, FLOATING_TEXT_MOB_HEIGHT);
      }
      worldPoint.y += (1 - entry.ttlMs / 1100) * 1.8;

      labelEntries.set(`floating:${entry.id}`, {
        text: entry.text,
        color: entry.color,
        className: 'floating-number',
        worldPoint,
        opacity: Math.min(entry.ttlMs / 250, 1),
        anchorTransform: '',
      });
    }

    for (const key of Array.from(this.labelNodes.keys())) {
      if (labelEntries.has(key)) {
        continue;
      }
      const node = this.labelNodes.get(key);
      if (node) {
        node.remove();
      }
      this.labelNodes.delete(key);
    }

    for (const [key, entry] of labelEntries) {
      let node = this.labelNodes.get(key);
      if (!node) {
        node = document.createElement('div');
        this.labelNodes.set(key, node);
        this.labelsHost.appendChild(node);
      }
      node.className = entry.className;
      node.textContent = entry.text;
      node.style.color = entry.color;

      const projected = entry.worldPoint.clone().project(this.camera);
      const inFrontOfCamera = projected.z >= -1 && projected.z <= 1;
      if (!inFrontOfCamera) {
        node.style.transform = 'translate(-9999px, -9999px)';
        node.style.opacity = '0';
        continue;
      }
      const x = Math.round(((projected.x + 1) / 2) * this.root.clientWidth);
      const y =
        Math.round(((-projected.y + 1) / 2) * this.root.clientHeight) +
        (entry.className === 'character-nameplate' ? PLAYER_NAMEPLATE_SCREEN_OFFSET_Y : 0);
      node.style.transform = entry.anchorTransform
        ? `${entry.anchorTransform} translate(${x}px, ${y}px)`
        : `translate(${x}px, ${y}px)`;
      node.style.opacity = `${entry.opacity}`;
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
