import * as THREE from 'three';
import type { Object3D } from 'three';
import type { CharacterSummary } from '../online/contracts';

type AppearanceOptionIndex = 0 | 1 | 2;

type CharacterRigPart = {
  object: Object3D;
  position: THREE.Vector3;
  rotation: THREE.Euler;
};

type ProceduralCharacterRig = {
  torso: CharacterRigPart;
  head: CharacterRigPart;
  hair: CharacterRigPart;
  armLeft: CharacterRigPart;
  armRight: CharacterRigPart;
  legLeft: CharacterRigPart;
  legRight: CharacterRigPart;
  weapon: CharacterRigPart;
};

type LobbyCharacterVisual = {
  character: CharacterSummary;
  group: THREE.Group;
  startPosition: THREE.Vector3;
  targetPosition: THREE.Vector3;
  startedAtMs: number;
  selected: boolean;
  interactionStartedAtMs: number | null;
};

type CharacterVisualOptions = {
  showName?: boolean;
};

const SELECTED_POSITION = new THREE.Vector3(0, 0, -1.45);
const WALK_DURATION_MS = 1150;
const INTERACTION_DURATION_MS = 920;
const ROSTER_POSITIONS = [
  new THREE.Vector3(-7.3, 0, 0.3),
  new THREE.Vector3(-4.4, 0, -0.35),
  new THREE.Vector3(4.35, 0, -0.35),
  new THREE.Vector3(7.15, 0, 0.25),
  new THREE.Vector3(-9.4, 0, 1.15),
  new THREE.Vector3(9.3, 0, 1.1),
];

const easeOutCubic = (value: number): number => 1 - Math.pow(1 - value, 3);

const requireAppearanceIndex = (value: number, field: string): AppearanceOptionIndex => {
  if (value === 0 || value === 1 || value === 2) {
    return value;
  }
  throw new Error(`Invalid canonical ${field}: ${value}`);
};

const characterSkin = (character: CharacterSummary): string => {
  const face = requireAppearanceIndex(character.face, 'face');
  const palettes: Record<CharacterSummary['race'], readonly [string, string, string]> = {
    Human: ['#d8b99f', '#cfaa91', '#e0c7ad'],
    Elf: ['#e5d2b5', '#dfc9a7', '#f0dfc0'],
    'Dark Elf': ['#8c8da5', '#767992', '#9da0b6'],
    Orc: ['#78906d', '#6d8664', '#879b75'],
    Dwarf: ['#c0926f', '#ad7c58', '#d3a27b'],
  };
  return palettes[character.race][face];
};

const gearTint = (character: CharacterSummary): string => {
  if (character.base_class === 'Mage') {
    return character.race === 'Dark Elf' ? '#35405e' : '#52617b';
  }
  return character.race === 'Orc' ? '#4e5948' : '#5a5f67';
};

const hairTint = (character: CharacterSummary): string => {
  const hairColor = requireAppearanceIndex(character.hair_color, 'hair_color');
  const palettes: Record<CharacterSummary['race'], readonly [string, string, string]> = {
    Human: ['#6b4e37', '#c5a46a', '#26211c'],
    Elf: ['#e4d47d', '#fff0a8', '#c6c3b4'],
    'Dark Elf': ['#e7eef0', '#c5d4dc', '#f4f4ff'],
    Orc: ['#1d201c', '#44372d', '#2e3026'],
    Dwarf: ['#b66d3d', '#e0a65d', '#f1f1e6'],
  };
  return palettes[character.race][hairColor];
};

const createStandardMaterial = (color: string, roughness = 0.82): THREE.MeshStandardMaterial =>
  new THREE.MeshStandardMaterial({ color, roughness });

const tagCharacterObject = (object: Object3D, characterId: string): void => {
  object.userData = { ...object.userData, characterId };
  for (const child of object.children) {
    tagCharacterObject(child, characterId);
  }
};

const captureRigPart = (object: Object3D): CharacterRigPart => ({
  object,
  position: object.position.clone(),
  rotation: object.rotation.clone(),
});

const applyRigPartBase = (part: CharacterRigPart): void => {
  part.object.position.copy(part.position);
  part.object.rotation.copy(part.rotation);
};

export const animateCharacterVisual = (
  group: THREE.Group,
  timeMs: number,
  options: {
    moving?: boolean;
    interactionProgress?: number | null;
    phaseOffset?: number;
  } = {},
): void => {
  const rig = group.userData.proceduralRig as ProceduralCharacterRig | undefined;
  if (!rig) {
    return;
  }

  for (const part of Object.values(rig)) {
    applyRigPartBase(part);
  }

  const offset = options.phaseOffset ?? group.id * 0.37;
  const idle = Math.sin(timeMs * 0.002 + offset);
  const walkPhase = timeMs * 0.011 + offset;
  const swing = Math.sin(walkPhase);
  const counterSwing = Math.cos(walkPhase * 2);
  const movingAmount = options.moving ? 1 : 0;
  const idleAmount = options.moving ? 0 : 1;

  rig.torso.object.rotation.z += counterSwing * 0.028 * movingAmount + idle * 0.012 * idleAmount;
  rig.head.object.position.y += idle * 0.025 * idleAmount;
  rig.hair.object.position.y += idle * 0.024 * idleAmount;
  rig.legLeft.object.rotation.x += -swing * 0.62 * movingAmount;
  rig.legRight.object.rotation.x += swing * 0.62 * movingAmount;
  rig.armLeft.object.rotation.x += swing * 0.65 * movingAmount;
  rig.armRight.object.rotation.x += -swing * 0.65 * movingAmount;
  rig.armLeft.object.rotation.z += 0.08;
  rig.armRight.object.rotation.z -= 0.08;

  if (options.interactionProgress !== null && options.interactionProgress !== undefined) {
    const wave = Math.sin(options.interactionProgress * Math.PI * 6);
    const settle = 1 - options.interactionProgress;
    rig.armLeft.object.rotation.x = -1.05 + wave * 0.22 * settle;
    rig.armRight.object.rotation.x = -1.05 - wave * 0.22 * settle;
    rig.armLeft.object.rotation.z = 0.62;
    rig.armRight.object.rotation.z = -0.62;
  }
};

const createNameSprite = (name: string): THREE.Sprite => {
  const canvas = document.createElement('canvas');
  canvas.width = 256;
  canvas.height = 64;
  const context = canvas.getContext('2d');
  if (context) {
    context.clearRect(0, 0, canvas.width, canvas.height);
    context.font = '700 23px Arial';
    context.textAlign = 'center';
    context.textBaseline = 'middle';
    context.lineWidth = 5;
    context.strokeStyle = 'rgba(0, 0, 0, 0.92)';
    context.strokeText(name, canvas.width / 2, canvas.height / 2);
    context.fillStyle = '#f4f7ff';
    context.fillText(name, canvas.width / 2, canvas.height / 2);
  }
  const texture = new THREE.CanvasTexture(canvas);
  texture.colorSpace = THREE.SRGBColorSpace;
  const material = new THREE.SpriteMaterial({ map: texture, transparent: true });
  const sprite = new THREE.Sprite(material);
  sprite.userData = { ...sprite.userData, lobbyNameSprite: true };
  sprite.scale.set(2.35, 0.58, 1);
  sprite.position.y = 3.75;
  return sprite;
};

const createBanner = (x: number, color: string): THREE.Group => {
  const group = new THREE.Group();
  const cloth = new THREE.Mesh(
    new THREE.PlaneGeometry(1.2, 3.8),
    new THREE.MeshStandardMaterial({
      color,
      roughness: 0.95,
      side: THREE.DoubleSide,
    }),
  );
  cloth.position.set(x, 3.4, -7.72);
  const trim = new THREE.Mesh(new THREE.BoxGeometry(1.35, 0.08, 0.08), createStandardMaterial('#b39a58', 0.7));
  trim.position.set(x, 5.35, -7.68);
  group.add(cloth, trim);
  return group;
};

const createTorch = (x: number, z: number): THREE.Group => {
  const group = new THREE.Group();
  const bracket = new THREE.Mesh(new THREE.CylinderGeometry(0.06, 0.06, 0.85, 6), createStandardMaterial('#392a20', 0.9));
  bracket.rotation.z = Math.PI / 2;
  bracket.position.set(x, 2.65, z);
  const bowl = new THREE.Mesh(new THREE.CylinderGeometry(0.18, 0.24, 0.22, 7), createStandardMaterial('#4d3924', 0.82));
  bowl.position.set(x, 2.65, z + 0.35);
  const flame = new THREE.Mesh(
    new THREE.ConeGeometry(0.23, 0.8, 7),
    new THREE.MeshBasicMaterial({ color: '#ffbb68', transparent: true, opacity: 0.92 }),
  );
  flame.position.set(x, 3.17, z + 0.35);
  const light = new THREE.PointLight('#ffa55d', 1.15, 9);
  light.position.set(x, 3.2, z + 0.45);
  group.add(bracket, bowl, flame, light);
  return group;
};

export const createCharacterVisual = (
  character: CharacterSummary,
  options: CharacterVisualOptions = {},
): THREE.Group => {
  const group = new THREE.Group();
  const isMage = character.base_class === 'Mage';
  const isDwarf = character.race === 'Dwarf';
  const isOrc = character.race === 'Orc';
  const isFemale = character.sex === 'Female';
  const heightScale = isDwarf ? 0.82 : isOrc ? 1.12 : isFemale ? 0.96 : 1;
  const widthScale = isOrc ? 1.18 : isDwarf ? 1.05 : isFemale ? 0.88 : 1;

  const skinMaterial = createStandardMaterial(characterSkin(character), 0.78);
  const gearMaterial = createStandardMaterial(gearTint(character), 0.8);
  const darkGearMaterial = createStandardMaterial(isMage ? '#20283d' : '#292b2d', 0.9);
  const bootMaterial = createStandardMaterial('#252025', 0.9);

  const legs = new THREE.Group();
  const legGeometry = new THREE.BoxGeometry(0.33 * widthScale, 1.1 * heightScale, 0.34 * widthScale);
  const legMeshes: THREE.Mesh[] = [];
  for (const x of [-0.26 * widthScale, 0.26 * widthScale]) {
    const leg = new THREE.Mesh(legGeometry, isMage ? darkGearMaterial : bootMaterial);
    leg.position.set(x, 0.58 * heightScale, 0);
    legMeshes.push(leg);
    legs.add(leg);
  }

  const torsoGeometry = isMage
    ? new THREE.CylinderGeometry(0.58 * widthScale, 0.8 * widthScale, 1.72 * heightScale, 7)
    : new THREE.BoxGeometry(1.05 * widthScale, 1.45 * heightScale, 0.64 * widthScale);
  const torso = new THREE.Mesh(torsoGeometry, gearMaterial);
  torso.position.y = 1.72 * heightScale;

  const chestPlate = new THREE.Mesh(
    new THREE.BoxGeometry(0.82 * widthScale, 0.84 * heightScale, 0.08),
    createStandardMaterial(isMage ? '#1c2542' : '#777b80', 0.62),
  );
  chestPlate.position.set(0, 1.96 * heightScale, 0.36 * widthScale);

  const head = new THREE.Mesh(new THREE.SphereGeometry(0.38 * widthScale, 12, 10), skinMaterial);
  head.position.y = 2.78 * heightScale;

  const hairStyle = requireAppearanceIndex(character.hair_style, 'hair_style');
  const hair = new THREE.Mesh(
    new THREE.SphereGeometry(
      (0.4 + hairStyle * 0.025) * widthScale,
      12,
      8,
      0,
      Math.PI * 2,
      0,
      Math.PI * (0.48 + hairStyle * 0.06),
    ),
    createStandardMaterial(hairTint(character), 0.85),
  );
  hair.position.set(0, (2.88 - hairStyle * 0.015) * heightScale, -0.03);

  const eyeMaterial = new THREE.MeshBasicMaterial({ color: '#17120f' });
  const leftEye = new THREE.Mesh(new THREE.BoxGeometry(0.055, 0.04, 0.018), eyeMaterial);
  leftEye.position.set(-0.12 * widthScale, 2.82 * heightScale, 0.36 * widthScale);
  const rightEye = leftEye.clone();
  rightEye.position.x = 0.12 * widthScale;
  const mouth = new THREE.Mesh(new THREE.BoxGeometry(0.16, 0.026, 0.018), eyeMaterial);
  mouth.position.set(0, 2.66 * heightScale, 0.36 * widthScale);

  const armGeometry = new THREE.BoxGeometry(0.25 * widthScale, 1.04 * heightScale, 0.26 * widthScale);
  const leftArm = new THREE.Mesh(armGeometry, skinMaterial);
  leftArm.position.set(-0.78 * widthScale, 1.73 * heightScale, 0);
  leftArm.rotation.z = 0.08;
  const rightArm = new THREE.Mesh(armGeometry, skinMaterial);
  rightArm.position.set(0.78 * widthScale, 1.73 * heightScale, 0);
  rightArm.rotation.z = -0.08;

  const shoulderGeometry = new THREE.BoxGeometry(0.44 * widthScale, 0.22 * heightScale, 0.58 * widthScale);
  const shoulderLeft = new THREE.Mesh(shoulderGeometry, createStandardMaterial(isMage ? '#3a435e' : '#7a6a52', 0.74));
  shoulderLeft.position.set(-0.69 * widthScale, 2.28 * heightScale, 0);
  const shoulderRight = shoulderLeft.clone();
  shoulderRight.position.x = 0.69 * widthScale;

  const weapon = new THREE.Group();
  if (isMage) {
    const staff = new THREE.Mesh(new THREE.CylinderGeometry(0.045, 0.045, 2.5 * heightScale, 6), createStandardMaterial('#826245', 0.85));
    staff.position.set(0.95 * widthScale, 1.55 * heightScale, 0.08);
    staff.rotation.z = -0.15;
    const gem = new THREE.Mesh(new THREE.OctahedronGeometry(0.17, 0), new THREE.MeshBasicMaterial({ color: '#79a8ff' }));
    gem.position.set(0.72 * widthScale, 2.72 * heightScale, 0.08);
    weapon.add(staff, gem);
  } else {
    const blade = new THREE.Mesh(new THREE.BoxGeometry(0.11, 1.58 * heightScale, 0.07), createStandardMaterial('#cfd4d8', 0.38));
    blade.position.set(0.92 * widthScale, 1.85 * heightScale, 0.08);
    blade.rotation.z = -0.28;
    const hilt = new THREE.Mesh(new THREE.BoxGeometry(0.42, 0.08, 0.08), createStandardMaterial('#9f7b38', 0.6));
    hilt.position.set(0.72 * widthScale, 1.16 * heightScale, 0.08);
    weapon.add(blade, hilt);
  }

  group.add(
    legs,
    torso,
    chestPlate,
    head,
    hair,
    leftEye,
    rightEye,
    mouth,
    leftArm,
    rightArm,
    shoulderLeft,
    shoulderRight,
    weapon,
  );
  if (options.showName !== false) {
    group.add(createNameSprite(character.name));
  }
  group.scale.setScalar(1.04);
  tagCharacterObject(group, character.character_id);
  const [leftLeg, rightLeg] = legMeshes as [THREE.Mesh, THREE.Mesh];
  group.userData.proceduralRig = {
    torso: captureRigPart(torso),
    head: captureRigPart(head),
    hair: captureRigPart(hair),
    armLeft: captureRigPart(leftArm),
    armRight: captureRigPart(rightArm),
    legLeft: captureRigPart(leftLeg),
    legRight: captureRigPart(rightLeg),
    weapon: captureRigPart(weapon),
  } satisfies ProceduralCharacterRig;
  return group;
};

export class CharacterLobbyScene {
  private readonly renderer: THREE.WebGLRenderer;
  private readonly scene = new THREE.Scene();
  private readonly camera = new THREE.PerspectiveCamera(42, 1, 0.1, 120);
  private readonly raycaster = new THREE.Raycaster();
  private readonly pointer = new THREE.Vector2();
  private readonly visuals = new Map<string, LobbyCharacterVisual>();
  private frameHandle = 0;
  private destroyed = false;

  constructor(
    private readonly host: HTMLElement,
    characters: CharacterSummary[],
    private readonly selectedCharacterId: string | null,
    private readonly onSelectCharacter: (characterId: string) => void,
  ) {
    this.renderer = new THREE.WebGLRenderer({ antialias: true, alpha: true });
    this.renderer.setPixelRatio(Math.min(window.devicePixelRatio, 2));
    this.renderer.shadowMap.enabled = true;
    this.renderer.shadowMap.type = THREE.PCFSoftShadowMap;
    this.host.replaceChildren(this.renderer.domElement);
    this.scene.background = new THREE.Color('#171315');
    this.scene.fog = new THREE.Fog('#171315', 14, 35);
    this.camera.position.set(0, 3.6, 10.6);
    this.camera.lookAt(0, 1.55, -1.65);
    this.createHall();
    this.createCharacters(characters);
    this.renderer.domElement.addEventListener('pointerdown', this.handlePointerDown);
    window.addEventListener('resize', this.handleResize);
    this.handleResize();
    this.animate(performance.now());
  }

  destroy(): void {
    this.destroyed = true;
    cancelAnimationFrame(this.frameHandle);
    this.renderer.domElement.removeEventListener('pointerdown', this.handlePointerDown);
    window.removeEventListener('resize', this.handleResize);
    this.scene.traverse((object) => {
      const mesh = object as THREE.Mesh;
      mesh.geometry?.dispose();
      const material = mesh.material;
      if (Array.isArray(material)) {
        material.forEach((entry) => entry.dispose());
      } else {
        material?.dispose();
      }
    });
    this.renderer.dispose();
    this.host.replaceChildren();
  }

  private createHall(): void {
    const ambient = new THREE.HemisphereLight('#d5d1bb', '#1b1412', 1.25);
    this.scene.add(ambient);

    const sun = new THREE.DirectionalLight('#ffe1b0', 2.15);
    sun.position.set(6.2, 10.5, 3.8);
    sun.castShadow = true;
    sun.shadow.mapSize.set(1024, 1024);
    this.scene.add(sun);

    const floor = new THREE.Mesh(
      new THREE.PlaneGeometry(26, 17),
      createStandardMaterial('#4a3c32', 0.94),
    );
    floor.rotation.x = -Math.PI / 2;
    floor.position.z = -0.2;
    floor.receiveShadow = true;
    this.scene.add(floor);

    const tileMaterial = createStandardMaterial('#2e2927', 0.96);
    for (let x = -12; x <= 12; x += 2) {
      const line = new THREE.Mesh(new THREE.BoxGeometry(0.025, 0.012, 17), tileMaterial);
      line.position.set(x, 0.012, -0.2);
      this.scene.add(line);
    }
    for (let z = -8; z <= 8; z += 2) {
      const line = new THREE.Mesh(new THREE.BoxGeometry(26, 0.012, 0.025), tileMaterial);
      line.position.set(0, 0.014, z);
      this.scene.add(line);
    }

    const backWall = new THREE.Mesh(new THREE.BoxGeometry(26, 7.2, 0.45), createStandardMaterial('#514a4b', 0.97));
    backWall.position.set(0, 3.55, -8.1);
    backWall.receiveShadow = true;
    this.scene.add(backWall);

    const dais = new THREE.Mesh(new THREE.BoxGeometry(10, 0.45, 2.2), createStandardMaterial('#473c39', 0.92));
    dais.position.set(0, 0.23, -6.55);
    this.scene.add(dais);

    const door = new THREE.Mesh(new THREE.BoxGeometry(2.3, 4.3, 0.22), createStandardMaterial('#2f2526', 0.86));
    door.position.set(0, 2.55, -7.78);
    this.scene.add(door);

    const crest = new THREE.Mesh(new THREE.TorusGeometry(0.72, 0.045, 8, 32), createStandardMaterial('#b19756', 0.5));
    crest.position.set(0, 3.35, -7.6);
    this.scene.add(crest);

    for (const x of [-10.5, -7.6, -4.8, 4.8, 7.6, 10.5]) {
      const column = new THREE.Group();
      const shaft = new THREE.Mesh(new THREE.CylinderGeometry(0.34, 0.45, 6.3, 8), createStandardMaterial('#5f5656', 0.98));
      shaft.position.y = 3.15;
      const capTop = new THREE.Mesh(new THREE.BoxGeometry(1.1, 0.35, 0.8), createStandardMaterial('#6b6160', 0.92));
      capTop.position.y = 6.4;
      const capBottom = capTop.clone();
      capBottom.position.y = 0.2;
      column.add(shaft, capTop, capBottom);
      column.position.set(x, 0, -7.15);
      this.scene.add(column);
    }

    this.scene.add(createBanner(-1.15, '#2d6f72'), createBanner(1.15, '#2d6f72'));
    this.scene.add(createBanner(-6.2, '#603d3d'), createBanner(6.2, '#603d3d'));
    this.scene.add(createTorch(-11.1, -5.85), createTorch(-8.6, -5.95), createTorch(8.8, -5.9), createTorch(11.1, -5.85));

    for (let index = 0; index < 5; index++) {
      const beam = new THREE.Mesh(
        new THREE.PlaneGeometry(2.9, 14),
        new THREE.MeshBasicMaterial({
          color: '#ffdba8',
          transparent: true,
          opacity: 0.08,
          depthWrite: false,
          side: THREE.DoubleSide,
        }),
      );
      beam.position.set(4.5 + index * 1.15, 4.2, -1.4 + index * 0.35);
      beam.rotation.set(-0.55, 0.05, -0.45);
      this.scene.add(beam);
    }

    const circle = new THREE.Mesh(
      new THREE.RingGeometry(1.65, 2.05, 64),
      new THREE.MeshBasicMaterial({ color: '#b9964e', transparent: true, opacity: 0.62, side: THREE.DoubleSide }),
    );
    circle.rotation.x = -Math.PI / 2;
    circle.position.set(0, 0.035, -1.45);
    this.scene.add(circle);
  }

  private createCharacters(characters: CharacterSummary[]): void {
    const now = performance.now();
    characters.slice(0, 6).forEach((character, index) => {
      const selected = character.character_id === this.selectedCharacterId;
      const rosterPosition = ROSTER_POSITIONS[index] ?? ROSTER_POSITIONS[ROSTER_POSITIONS.length - 1];
      const group = createCharacterVisual(character);
      const targetPosition = selected ? SELECTED_POSITION.clone() : rosterPosition.clone();
      const startPosition = selected ? rosterPosition.clone() : targetPosition.clone();
      group.position.copy(startPosition);
      group.rotation.y = selected ? 0 : Math.atan2(SELECTED_POSITION.x - rosterPosition.x, SELECTED_POSITION.z - rosterPosition.z);
      group.castShadow = true;
      group.traverse((object) => {
        const mesh = object as THREE.Mesh;
        if (mesh.isMesh) {
          mesh.castShadow = true;
          mesh.receiveShadow = true;
        }
      });
      this.visuals.set(character.character_id, {
        character,
        group,
        startPosition,
        targetPosition,
        startedAtMs: now,
        selected,
        interactionStartedAtMs: null,
      });
      this.scene.add(group);
    });
  }

  private readonly handleResize = (): void => {
    const width = Math.max(this.host.clientWidth, 1);
    const height = Math.max(this.host.clientHeight, 1);
    this.camera.aspect = width / height;
    this.camera.updateProjectionMatrix();
    this.renderer.setSize(width, height, false);
  };

  private readonly handlePointerDown = (event: PointerEvent): void => {
    const bounds = this.renderer.domElement.getBoundingClientRect();
    this.pointer.x = ((event.clientX - bounds.left) / bounds.width) * 2 - 1;
    this.pointer.y = -((event.clientY - bounds.top) / bounds.height) * 2 + 1;
    this.raycaster.setFromCamera(this.pointer, this.camera);
    const hit = this.raycaster.intersectObjects(this.scene.children, true).find((entry) => this.resolveCharacterId(entry.object));
    const characterId = hit ? this.resolveCharacterId(hit.object) : null;
    if (characterId) {
      const visual = this.visuals.get(characterId);
      if (visual?.selected) {
        this.triggerSelectedCharacterInteraction(visual, performance.now());
        return;
      }
      this.onSelectCharacter(characterId);
    }
  };

  private triggerSelectedCharacterInteraction(visual: LobbyCharacterVisual, now: number): void {
    const walkProgress = Math.min(Math.max((now - visual.startedAtMs) / WALK_DURATION_MS, 0), 1);
    if (walkProgress < 1) {
      return;
    }
    visual.interactionStartedAtMs = now;
  }

  private resolveCharacterId(object: Object3D | null): string | null {
    let current: Object3D | null = object;
    while (current) {
      const characterId = current.userData.characterId;
      if (typeof characterId === 'string') {
        return characterId;
      }
      current = current.parent;
    }
    return null;
  }

  private animate = (now: number): void => {
    if (this.destroyed) {
      return;
    }
    for (const visual of this.visuals.values()) {
      const rawProgress = Math.min(Math.max((now - visual.startedAtMs) / WALK_DURATION_MS, 0), 1);
      const progress = easeOutCubic(rawProgress);
      visual.group.position.lerpVectors(visual.startPosition, visual.targetPosition, progress);
      const lookAt = visual.selected ? this.camera.position : SELECTED_POSITION;
      visual.group.rotation.y = Math.atan2(lookAt.x - visual.group.position.x, lookAt.z - visual.group.position.z);
      const isWalkingToCenter = visual.selected && rawProgress < 1;
      const interactionProgress =
        visual.selected && visual.interactionStartedAtMs !== null
          ? Math.min((now - visual.interactionStartedAtMs) / INTERACTION_DURATION_MS, 1)
          : null;
      animateCharacterVisual(visual.group, now, {
        moving: isWalkingToCenter,
        interactionProgress,
        phaseOffset: visual.character.character_id.length * 0.13,
      });
      if (visual.selected && rawProgress < 1) {
        visual.group.position.y = Math.sin(rawProgress * Math.PI * 7) * 0.025;
      }
      if (visual.selected && visual.interactionStartedAtMs !== null) {
        const activeInteractionProgress = Math.min((now - visual.interactionStartedAtMs) / INTERACTION_DURATION_MS, 1);
        const hop = Math.max(0, Math.sin(activeInteractionProgress * Math.PI * 6));
        const settle = 1 - easeOutCubic(activeInteractionProgress);
        visual.group.position.y = hop * (0.34 + settle * 0.1);
        visual.group.rotation.z = Math.sin(activeInteractionProgress * Math.PI * 6) * 0.045 * settle;
        if (activeInteractionProgress >= 1) {
          visual.interactionStartedAtMs = null;
          visual.group.position.y = 0;
          visual.group.rotation.z = 0;
        }
      }
    }
    this.renderer.render(this.scene, this.camera);
    this.frameHandle = requestAnimationFrame(this.animate);
  };
}
