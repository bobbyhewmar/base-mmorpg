import * as THREE from 'three';
import { FBXLoader } from 'three/examples/jsm/loaders/FBXLoader.js';
import { GLTFLoader, type GLTF } from 'three/examples/jsm/loaders/GLTFLoader.js';
import { getBaseClassDefinition, getCanonicalGltfAssetCatalogEntry } from '../data/characterClasses';
import type { BaseClass, CharacterSex } from '../domain/types';
import { loadCatalogedGltf } from './gltfExternalAssets';

type ClassCharacterModelActionName = 'idle' | 'walk' | 'run';

type MixerFinishedEvent = {
  type: 'finished';
  action: THREE.AnimationAction;
  direction: number;
};

type ClassCharacterModelRuntime = {
  key: string;
  container: THREE.Group;
  root: THREE.Group | null;
  mixer: THREE.AnimationMixer | null;
  actions: Partial<Record<ClassCharacterModelActionName, THREE.AnimationAction>>;
  currentAction: ClassCharacterModelActionName | null;
  lobbyInteractionActions: THREE.AnimationAction[];
  activeLobbyInteractionAction: THREE.AnimationAction | null;
  handleMixerFinished: ((event: MixerFinishedEvent) => void) | null;
  proceduralRoot?: THREE.Object3D;
};

export type ClassCharacterModelAppearance = {
  baseClass: BaseClass;
  sex: CharacterSex;
  hairStyle: number;
  hairColor: string;
  skinType: number;
};

export const CLASS_CHARACTER_MODEL_RUNTIME_KEY = 'classCharacterModelRuntime';
const fbxLoader = new FBXLoader();
const gltfLoader = new GLTFLoader();
const textureLoader = new THREE.TextureLoader();
const textureCache = new Map<string, Promise<THREE.Texture>>();
const animationCache = new Map<string, Promise<THREE.AnimationClip | null>>();
const animationListCache = new Map<string, Promise<THREE.AnimationClip[]>>();
const canonicalGltfCache = new Map<string, Promise<GLTF>>();

const appearanceOptionIndex = (value: number): 0 | 1 | 2 => {
  if (value === 0 || value === 1 || value === 2) {
    return value;
  }
  return 0;
};

const modelKeyFor = (appearance: ClassCharacterModelAppearance, desiredHeight: number): string =>
  `${appearance.baseClass}:${appearance.sex}:${appearanceOptionIndex(appearance.hairStyle)}:${appearance.hairColor}:${appearanceOptionIndex(appearance.skinType)}:${desiredHeight.toFixed(2)}`;

const disposeMaterial = (material: THREE.Material | THREE.Material[]): void => {
  if (Array.isArray(material)) {
    material.forEach((entry) => entry.dispose());
    return;
  }
  material.dispose();
};

const disposeObjectTree = (object: THREE.Object3D): void => {
  object.traverse((entry) => {
    const mesh = entry as THREE.Mesh;
    mesh.geometry?.dispose();
    if (mesh.material) {
      disposeMaterial(mesh.material);
    }
  });
};

const loadTexture = (url: string): Promise<THREE.Texture> => {
  const cached = textureCache.get(url);
  if (cached) {
    return cached;
  }
  const promise = textureLoader.loadAsync(url).then((texture) => {
    texture.colorSpace = THREE.SRGBColorSpace;
    texture.wrapS = THREE.ClampToEdgeWrapping;
    texture.wrapT = THREE.ClampToEdgeWrapping;
    texture.needsUpdate = true;
    return texture;
  });
  textureCache.set(url, promise);
  return promise;
};

const loadCanonicalGltf = (url: string): Promise<GLTF> => {
  const cached = canonicalGltfCache.get(url);
  if (cached) {
    return cached;
  }
  const promise = loadCatalogedGltf(gltfLoader, url, getCanonicalGltfAssetCatalogEntry);
  canonicalGltfCache.set(url, promise);
  return promise;
};

export const selectAnimationClip = (clips: THREE.AnimationClip[], preferredName: string): THREE.AnimationClip | null => {
  const normalizedPreferredName = preferredName.toLowerCase();
  return (
    clips.find((clip) => clip.name.toLowerCase() === normalizedPreferredName) ??
    clips.find((clip) => clip.name.toLowerCase().endsWith(`|${normalizedPreferredName}`)) ??
    clips.find((clip) => clip.name.toLowerCase().includes(normalizedPreferredName)) ??
    null
  );
};

export const stripBoneScaleTracks = (clip: THREE.AnimationClip): THREE.AnimationClip => {
  const tracks = clip.tracks.filter((track) => !track.name.toLowerCase().endsWith('.scale'));
  return new THREE.AnimationClip(clip.name, clip.duration, tracks, clip.blendMode);
};

const normalizeAnimationClipName = (name: string): string =>
  name
    .toLowerCase()
    .replace(/^.*\|/, '')
    .replace(/[_\-.]+/g, ' ')
    .replace(/\s+/g, ' ')
    .trim();

const locomotionAnimationPatterns = [
  'walk',
  'run',
  'jog',
  'strafe',
  'sprint',
  'locomotion',
  'turn',
  'rotate',
  'backpedal',
  'backward',
  'back walk',
  'forward',
  'fwd',
  'movement',
] as const;

const isLocomotionAnimationName = (name: string): boolean => {
  const normalized = normalizeAnimationClipName(name);
  return locomotionAnimationPatterns.some((pattern) => normalized.includes(pattern));
};

export const selectLobbyInteractionClips = (
  clips: THREE.AnimationClip[],
  locomotionClipNames: string[],
): THREE.AnimationClip[] => {
  const excludedNames = new Set(locomotionClipNames.map((name) => normalizeAnimationClipName(name)).filter(Boolean));
  return clips.filter((clip) => {
    const normalized = normalizeAnimationClipName(clip.name);
    return normalized.length > 0 && !excludedNames.has(normalized) && !isLocomotionAnimationName(clip.name);
  });
};

export const pickLobbyInteractionClip = (
  clips: THREE.AnimationClip[],
  randomValue: number = Math.random(),
): THREE.AnimationClip | null => {
  if (clips.length === 0) {
    return null;
  }
  const normalizedRandom = Number.isFinite(randomValue) ? Math.min(Math.max(randomValue, 0), 0.999999) : 0;
  return clips[Math.floor(normalizedRandom * clips.length)] ?? clips[0] ?? null;
};

const loadFbxAnimation = (url: string, preferredName: string): Promise<THREE.AnimationClip | null> => {
  const cacheKey = `fbx:${url}#${preferredName}`;
  const cached = animationCache.get(cacheKey);
  if (cached) {
    return cached;
  }
  const promise = fbxLoader.loadAsync(url).then((asset) => selectAnimationClip(asset.animations, preferredName));
  animationCache.set(cacheKey, promise);
  return promise;
};

const loadGltfAnimation = (url: string, preferredName: string): Promise<THREE.AnimationClip | null> => {
  const cacheKey = `gltf:${url}#${preferredName}`;
  const cached = animationCache.get(cacheKey);
  if (cached) {
    return cached;
  }
  const promise = gltfLoader
    .loadAsync(url)
    .then((asset) => {
      const clip = selectAnimationClip(asset.animations, preferredName);
      return clip ? stripBoneScaleTracks(clip) : null;
    });
  animationCache.set(cacheKey, promise);
  return promise;
};

const loadGltfAnimations = (url: string): Promise<THREE.AnimationClip[]> => {
  const cached = animationListCache.get(url);
  if (cached) {
    return cached;
  }
  const promise = gltfLoader.loadAsync(url).then((asset) => asset.animations.map((clip) => stripBoneScaleTracks(clip)));
  animationListCache.set(url, promise);
  return promise;
};

const fitModelToHeight = (root: THREE.Object3D, desiredHeight: number): void => {
  root.updateMatrixWorld(true);
  const box = new THREE.Box3().setFromObject(root);
  const size = box.getSize(new THREE.Vector3());
  if (size.y <= 0.0001) {
    return;
  }

  root.scale.multiplyScalar(desiredHeight / size.y);
  root.updateMatrixWorld(true);

  const fittedBox = new THREE.Box3().setFromObject(root);
  const center = fittedBox.getCenter(new THREE.Vector3());
  root.position.x -= center.x;
  root.position.z -= center.z;
  root.position.y -= fittedBox.min.y;
};

const prepareLoadedModel = (
  root: THREE.Group,
  options: { castShadow: boolean; receiveShadow: boolean },
): void => {
  root.traverse((entry) => {
    const mesh = entry as THREE.Mesh;
    if (!mesh.isMesh) {
      return;
    }
    mesh.castShadow = options.castShadow;
    mesh.receiveShadow = options.receiveShadow;
    if (mesh.material) {
      mesh.material = Array.isArray(mesh.material)
        ? mesh.material.map((material) => material.clone())
        : mesh.material.clone();
    }
  });
};

const materialListFor = (material: THREE.Material | THREE.Material[] | undefined): THREE.Material[] => {
  if (!material) {
    return [];
  }
  return Array.isArray(material) ? material : [material];
};

const applyHairColor = (root: THREE.Object3D, color: string): void => {
  const tint = new THREE.Color(color);
  root.traverse((entry) => {
    const mesh = entry as THREE.Mesh;
    if (!mesh.isMesh || !mesh.material) {
      return;
    }
    const meshName = mesh.name.toLowerCase();
    for (const material of materialListFor(mesh.material)) {
      const materialName = material.name.toLowerCase();
      if (!meshName.includes('hair') && !materialName.includes('hair')) {
        continue;
      }
      if ('color' in material && material.color instanceof THREE.Color) {
        material.color.copy(tint);
        material.transparent = false;
        material.opacity = 1;
        material.depthWrite = true;
        material.needsUpdate = true;
      }
    }
  });
};

const firstSkinnedMesh = (root: THREE.Object3D): THREE.SkinnedMesh | null => {
  let result: THREE.SkinnedMesh | null = null;
  root.traverse((entry) => {
    if (result) {
      return;
    }
    const skinned = entry as THREE.SkinnedMesh;
    if (skinned.isSkinnedMesh) {
      result = skinned;
    }
  });
  return result;
};

const attachRiggedHairToBaseSkeleton = (baseRoot: THREE.Group, hairRoot: THREE.Group): void => {
  const baseSkinnedMesh = firstSkinnedMesh(baseRoot);
  const hairSkinnedMesh = firstSkinnedMesh(hairRoot);
  if (!baseSkinnedMesh || !hairSkinnedMesh) {
    throw new Error('Canonical hair asset must contain a skinned mesh compatible with the base body skeleton.');
  }

  baseRoot.updateMatrixWorld(true);
  hairRoot.updateMatrixWorld(true);
  hairSkinnedMesh.updateMatrixWorld(true);

  const material = Array.isArray(hairSkinnedMesh.material)
    ? hairSkinnedMesh.material.map((entry) => entry.clone())
    : hairSkinnedMesh.material.clone();
  const hair = new THREE.SkinnedMesh(hairSkinnedMesh.geometry.clone(), material);
  hair.name = `CanonicalHair_${hairSkinnedMesh.name || 'mesh'}`;
  hair.castShadow = true;
  hair.receiveShadow = true;
  hair.matrixAutoUpdate = false;
  hair.matrix.copy(new THREE.Matrix4().copy(baseRoot.matrixWorld).invert().multiply(hairSkinnedMesh.matrixWorld));
  hair.bind(baseSkinnedMesh.skeleton, hairSkinnedMesh.bindMatrix.clone());
  baseRoot.add(hair);
};

const applyBaseCharacterSkin = (root: THREE.Object3D, tint: string): void => {
  const tintColor = new THREE.Color(tint);
  root.traverse((entry) => {
    const mesh = entry as THREE.Mesh;
    if (!mesh.isMesh) {
      return;
    }
    for (const material of materialListFor(mesh.material)) {
      if (!material.name.toLowerCase().includes('superhero')) {
        continue;
      }
      const skinnedMaterial = material as THREE.MeshStandardMaterial;
      skinnedMaterial.color.copy(tintColor);
      skinnedMaterial.transparent = false;
      skinnedMaterial.opacity = 1;
      skinnedMaterial.depthWrite = true;
      skinnedMaterial.needsUpdate = true;
    }
  });
};

const prepareTexturedFbxModel = (
  root: THREE.Group,
  texture: THREE.Texture,
  options: { castShadow: boolean; receiveShadow: boolean },
): void => {
  root.traverse((entry) => {
    const mesh = entry as THREE.Mesh;
    if (!mesh.isMesh) {
      return;
    }
    mesh.castShadow = options.castShadow;
    mesh.receiveShadow = options.receiveShadow;
    mesh.material = new THREE.MeshStandardMaterial({
      map: texture,
      roughness: 0.82,
      metalness: 0.04,
    });
  });
};

const tintLoadedModelMaterials = (root: THREE.Object3D, tint: string): void => {
  const tintColor = new THREE.Color(tint);
  root.traverse((entry) => {
    const mesh = entry as THREE.Mesh;
    if (!mesh.isMesh || !mesh.material) {
      return;
    }
    const materials = Array.isArray(mesh.material) ? mesh.material : [mesh.material];
    for (const material of materials) {
      const normalizedName = material.name.toLowerCase();
      if (
        normalizedName.includes('regular') ||
        normalizedName.includes('skin') ||
        normalizedName.includes('eye') ||
        normalizedName.includes('hair')
      ) {
        continue;
      }
      if ('color' in material && material.color instanceof THREE.Color) {
        material.color.multiply(tintColor);
      }
    }
  });
};

const removeExistingRuntime = (parent: THREE.Object3D): void => {
  const existing = parent.userData[CLASS_CHARACTER_MODEL_RUNTIME_KEY] as ClassCharacterModelRuntime | undefined;
  if (!existing) {
    return;
  }
  if (existing.mixer && existing.handleMixerFinished) {
    existing.mixer.removeEventListener('finished', existing.handleMixerFinished);
  }
  existing.mixer?.stopAllAction();
  existing.container.removeFromParent();
  disposeObjectTree(existing.container);
  delete parent.userData[CLASS_CHARACTER_MODEL_RUNTIME_KEY];
};

const clearLobbyInteractionAction = (runtime: ClassCharacterModelRuntime): void => {
  if (!runtime.activeLobbyInteractionAction) {
    return;
  }
  runtime.activeLobbyInteractionAction.fadeOut(0.12);
  runtime.activeLobbyInteractionAction.stop();
  runtime.activeLobbyInteractionAction = null;
};

const playModelAction = (runtime: ClassCharacterModelRuntime, nextActionName: ClassCharacterModelActionName): void => {
  if (runtime.currentAction === nextActionName) {
    return;
  }
  const nextAction = runtime.actions[nextActionName];
  if (!nextAction) {
    runtime.currentAction = nextActionName;
    return;
  }
  const previousAction = runtime.currentAction ? runtime.actions[runtime.currentAction] : null;
  nextAction.reset().fadeIn(0.16).play();
  previousAction?.fadeOut(0.16);
  runtime.currentAction = nextActionName;
};

export const ensureClassCharacterModel = (
  parent: THREE.Object3D,
  appearance: ClassCharacterModelAppearance,
  options: {
    desiredHeight: number;
    castShadow?: boolean;
    receiveShadow?: boolean;
    proceduralRoot?: THREE.Object3D;
  },
): void => {
  const key = modelKeyFor(appearance, options.desiredHeight);
  const existing = parent.userData[CLASS_CHARACTER_MODEL_RUNTIME_KEY] as ClassCharacterModelRuntime | undefined;
  if (existing?.key === key) {
    return;
  }

  removeExistingRuntime(parent);
  if (options.proceduralRoot) {
    options.proceduralRoot.visible = false;
  }

  const definition = getBaseClassDefinition(appearance.baseClass);
  const container = new THREE.Group();
  container.visible = false;
  parent.add(container);

  const runtime: ClassCharacterModelRuntime = {
    key,
    container,
    root: null,
    mixer: null,
    actions: {},
    currentAction: null,
    lobbyInteractionActions: [],
    activeLobbyInteractionAction: null,
    handleMixerFinished: null,
    proceduralRoot: options.proceduralRoot,
  };
  parent.userData[CLASS_CHARACTER_MODEL_RUNTIME_KEY] = runtime;

  const shadowOptions = {
    castShadow: options.castShadow ?? true,
    receiveShadow: options.receiveShadow ?? true,
  };

  const visual = definition.visual;
  let assetPromise: Promise<{
    model: THREE.Group;
    idleClip: THREE.AnimationClip | null;
    walkClip: THREE.AnimationClip | null;
    runClip: THREE.AnimationClip | null;
    lobbyInteractionClips: THREE.AnimationClip[];
  }>;

  if (visual.kind === 'gltf_base_character') {
    const skinType = appearanceOptionIndex(appearance.skinType);
    const skinTint = visual.skinTints[appearance.sex][skinType] ?? '#ffffff';
    const hairUrl = visual.hairModelUrls[appearance.sex][appearanceOptionIndex(appearance.hairStyle)];
    if (!hairUrl) {
      throw new Error(`Missing canonical hair asset for ${appearance.sex} hairstyle ${appearance.hairStyle}.`);
    }
    assetPromise = Promise.all([
      loadCanonicalGltf(visual.modelUrls[appearance.sex]),
      loadCanonicalGltf(hairUrl),
      loadGltfAnimation(visual.animationUrl, visual.idleClipName),
      loadGltfAnimation(visual.animationUrl, visual.walkClipName),
      loadGltfAnimation(visual.animationUrl, visual.runClipName),
      loadGltfAnimations(visual.animationUrl),
    ]).then(([modelAsset, hairAsset, idleClip, walkClip, runClip, allClips]) => {
      const model = modelAsset.scene;
      prepareLoadedModel(model, shadowOptions);
      applyBaseCharacterSkin(model, skinTint);
      prepareLoadedModel(hairAsset.scene, shadowOptions);
      attachRiggedHairToBaseSkeleton(model, hairAsset.scene);
      applyHairColor(model, appearance.hairColor);
      disposeObjectTree(hairAsset.scene);
      return {
        model,
        idleClip,
        walkClip,
        runClip,
        lobbyInteractionClips: selectLobbyInteractionClips(allClips, [
          visual.idleClipName,
          visual.walkClipName,
          visual.runClipName,
        ]),
      };
    });
  } else {
    assetPromise = Promise.all([
      fbxLoader.loadAsync(visual.modelUrl),
      loadTexture(visual.textures[appearance.sex]),
      loadFbxAnimation(visual.idleAnimationUrl, 'Idle'),
      loadFbxAnimation(visual.runAnimationUrl, 'Run'),
    ]).then(([model, texture, idleClip, runClip]) => {
      prepareTexturedFbxModel(model, texture, shadowOptions);
      return { model, idleClip, walkClip: null, runClip, lobbyInteractionClips: [] };
    });
  }

  void assetPromise
    .then(({ model, idleClip, walkClip, runClip, lobbyInteractionClips }) => {
      if (parent.userData[CLASS_CHARACTER_MODEL_RUNTIME_KEY] !== runtime) {
        disposeObjectTree(model);
        return;
      }
      fitModelToHeight(model, options.desiredHeight);
      container.add(model);
      runtime.root = model;
      runtime.mixer = new THREE.AnimationMixer(model);
      runtime.handleMixerFinished = (event: MixerFinishedEvent) => {
        if (event.action !== runtime.activeLobbyInteractionAction) {
          return;
        }
        clearLobbyInteractionAction(runtime);
        playModelAction(runtime, 'idle');
      };
      runtime.mixer.addEventListener('finished', runtime.handleMixerFinished);
      if (idleClip) {
        runtime.actions.idle = runtime.mixer.clipAction(idleClip);
      }
      if (walkClip) {
        runtime.actions.walk = runtime.mixer.clipAction(walkClip);
      }
      if (runClip) {
        runtime.actions.run = runtime.mixer.clipAction(runClip);
      }
      runtime.lobbyInteractionActions = lobbyInteractionClips.map((clip) => runtime.mixer!.clipAction(clip));
      if (runtime.proceduralRoot) {
        runtime.proceduralRoot.visible = false;
      }
      container.visible = true;
      playModelAction(runtime, 'idle');
    })
    .catch((error: unknown) => {
      console.error('Failed to load canonical character model.', error);
    });
};

export const triggerClassCharacterModelLobbyInteraction = (
  parent: THREE.Object3D,
  randomValue: number = Math.random(),
): boolean => {
  const runtime = parent.userData[CLASS_CHARACTER_MODEL_RUNTIME_KEY] as ClassCharacterModelRuntime | undefined;
  if (!runtime?.mixer || !runtime.root) {
    return false;
  }

  const selectedAction = pickLobbyInteractionClip(
    runtime.lobbyInteractionActions.map((action) => action.getClip()),
    randomValue,
  );
  if (!selectedAction) {
    clearLobbyInteractionAction(runtime);
    playModelAction(runtime, 'idle');
    return false;
  }

  const action =
    runtime.lobbyInteractionActions.find((candidate) => candidate.getClip() === selectedAction) ?? runtime.mixer.clipAction(selectedAction);

  clearLobbyInteractionAction(runtime);
  const previousAction = runtime.currentAction ? runtime.actions[runtime.currentAction] : null;
  action.reset();
  action.setLoop(THREE.LoopOnce, 1);
  action.clampWhenFinished = true;
  action.fadeIn(0.12);
  action.play();
  previousAction?.fadeOut(0.12);
  runtime.activeLobbyInteractionAction = action;
  runtime.currentAction = null;
  return true;
};

export const updateClassCharacterModelAnimation = (
  parent: THREE.Object3D,
  deltaMs: number,
  options: {
    moving?: boolean;
    dead?: boolean;
    casting?: boolean;
    basicAttacking?: boolean;
    movementMode?: 'run' | 'walk';
  } = {},
): void => {
  const runtime = parent.userData[CLASS_CHARACTER_MODEL_RUNTIME_KEY] as ClassCharacterModelRuntime | undefined;
  if (!runtime?.root) {
    return;
  }

  runtime.root.rotation.set(0, 0, 0);
  runtime.root.position.y = 0;
  if (options.dead) {
    clearLobbyInteractionAction(runtime);
    runtime.root.rotation.z = Math.PI * 0.5;
    runtime.root.position.y = 0.56;
    playModelAction(runtime, 'idle');
  } else {
    if (!runtime.activeLobbyInteractionAction) {
      playModelAction(runtime, options.moving ? options.movementMode ?? 'run' : 'idle');
    }
    if (options.casting) {
      runtime.root.rotation.x = Math.sin(performance.now() * 0.012) * 0.08;
    }
    if (options.basicAttacking) {
      runtime.root.rotation.y = Math.sin(performance.now() * 0.018) * 0.18;
    }
  }

  runtime.mixer?.update(Math.max(0, deltaMs) / 1000);
};
