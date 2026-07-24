import type { BaseClass, CharacterSex } from '../domain/types';

export const BASE_CLASSES = ['Fighter', 'Mage'] as const;

export type ClassCombatFamily = 'physical' | 'mystic';

export type CharacterClassDefinition = {
  id: BaseClass;
  label: string;
  creationLabel: string;
  description: string;
  combatFamily: ClassCombatFamily;
  visual:
    | {
        kind: 'fbx_textured';
        modelUrl: string;
        idleAnimationUrl: string;
        runAnimationUrl: string;
        textures: Record<CharacterSex, string>;
      }
    | {
        kind: 'gltf_base_character';
        modelUrls: Record<CharacterSex, string>;
        hairModelUrls: Record<CharacterSex, string[]>;
        skinTextureUrls: Record<CharacterSex, string[]>;
        skinTints: Record<CharacterSex, string[]>;
        animationUrl: string;
        idleClipName: string;
        walkClipName: string;
        runClipName: string;
      };
};

export type CanonicalGltfAssetCatalogEntry = {
  source: string;
  resources: Record<string, string>;
};

const universalBaseAssets = import.meta.glob('../../assets/characters/universal-base/{animations,base,hair}/**/*', {
  query: '?url',
  import: 'default',
  eager: true,
}) as Record<string, string>;
const universalBaseRawGltfAssets = import.meta.glob('../../assets/characters/universal-base/{base,hair}/**/*.gltf', {
  query: '?raw',
  import: 'default',
  eager: true,
}) as Record<string, string>;
const canonicalGltfAssetCatalog = new Map<string, CanonicalGltfAssetCatalogEntry>();

const universalBaseAsset = (path: string): string => {
  const normalizedPath = path.replace(/\\/g, '/');
  const key = `../../assets/characters/universal-base/${normalizedPath}`;
  const assetUrl = universalBaseAssets[key];
  if (!assetUrl) {
    throw new Error(`Missing canonical Universal Base Character asset: ${normalizedPath}`);
  }
  return assetUrl;
};

const canonicalGltfResourcesFor = (normalizedPath: string): Record<string, string> => {
  const lastSlashIndex = normalizedPath.lastIndexOf('/');
  const directory = lastSlashIndex >= 0 ? normalizedPath.slice(0, lastSlashIndex) : '';
  const directoryKeyPrefix = `../../assets/characters/universal-base/${directory ? `${directory}/` : ''}`;
  const resources: Record<string, string> = {};
  for (const [assetKey, assetUrl] of Object.entries(universalBaseAssets)) {
    if (!assetKey.startsWith(directoryKeyPrefix) || assetKey.endsWith('.gltf')) {
      continue;
    }
    const relativeName = assetKey.slice(directoryKeyPrefix.length);
    if (relativeName.includes('/')) {
      continue;
    }
    resources[relativeName] = assetUrl;
  }
  return resources;
};

const registerCanonicalGltfAsset = (path: string): string => {
  const normalizedPath = path.replace(/\\/g, '/');
  const assetUrl = universalBaseAsset(normalizedPath);
  const rawKey = `../../assets/characters/universal-base/${normalizedPath}`;
  const rawSource = universalBaseRawGltfAssets[rawKey];
  if (rawSource) {
    canonicalGltfAssetCatalog.set(assetUrl, {
      source: rawSource,
      resources: canonicalGltfResourcesFor(normalizedPath),
    });
  }
  return assetUrl;
};

export const getCanonicalGltfAssetCatalogEntry = (url: string): CanonicalGltfAssetCatalogEntry | null =>
  canonicalGltfAssetCatalog.get(url) ?? null;

const universalAnimationUrl = universalBaseAsset('animations/UAL1_Standard.glb');
const humanBaseModelUrls: Record<CharacterSex, string> = {
  Male: registerCanonicalGltfAsset('base/godot-ue/Superhero_Male_FullBody.gltf'),
  Female: registerCanonicalGltfAsset('base/godot-ue/Superhero_Female_FullBody.gltf'),
};
const humanHairModelUrls: Record<CharacterSex, string[]> = {
  Male: [
    registerCanonicalGltfAsset('hair/rigged-gltf/Hair_Buzzed.gltf'),
    registerCanonicalGltfAsset('hair/rigged-gltf/Hair_SimpleParted.gltf'),
    registerCanonicalGltfAsset('hair/rigged-gltf/Hair_Beard.gltf'),
  ],
  Female: [
    registerCanonicalGltfAsset('hair/rigged-gltf/Hair_BuzzedFemale.gltf'),
    registerCanonicalGltfAsset('hair/rigged-gltf/Hair_Buns.gltf'),
    registerCanonicalGltfAsset('hair/rigged-gltf/Hair_Long.gltf'),
  ],
};
const humanSkinTextureUrls: Record<CharacterSex, string[]> = {
  Male: [
    universalBaseAsset('base/textures/T_Superhero_Male_Ligh.png'),
    universalBaseAsset('base/textures/T_Superhero_Male_Ligh.png'),
    universalBaseAsset('base/textures/T_Superhero_Male_Dark.png'),
  ],
  Female: [
    universalBaseAsset('base/textures/T_Superhero_Female_Light_BaseColor.png'),
    universalBaseAsset('base/textures/T_Superhero_Female_Light_BaseColor.png'),
    universalBaseAsset('base/textures/T_Superhero_Female_Dark_BaseColor.png'),
  ],
};
const humanSkinTints: Record<CharacterSex, string[]> = {
  Male: ['#ffffff', '#d1a17f', '#ffffff'],
  Female: ['#ffffff', '#d5a589', '#ffffff'],
};

const universalHumanVisual = (
  runClipName: string,
): Extract<CharacterClassDefinition['visual'], { kind: 'gltf_base_character' }> => ({
  kind: 'gltf_base_character',
  modelUrls: humanBaseModelUrls,
  hairModelUrls: humanHairModelUrls,
  skinTextureUrls: humanSkinTextureUrls,
  skinTints: humanSkinTints,
  animationUrl: universalAnimationUrl,
  idleClipName: 'Idle_Loop',
  walkClipName: 'Walk_Loop',
  runClipName,
});

export const CHARACTER_CLASS_DEFINITIONS: Record<BaseClass, CharacterClassDefinition> = {
  Fighter: {
    id: 'Fighter',
    label: 'Fighter',
    creationLabel: 'Fighter',
    description: 'Close-range weapon pressure, armor, and direct target commitment.',
    combatFamily: 'physical',
    visual: universalHumanVisual('Jog_Fwd_Loop'),
  },
  Mage: {
    id: 'Mage',
    label: 'Mage',
    creationLabel: 'Mystic',
    description: 'Target-locked spellcasting, higher magical reserve, and slower area pressure.',
    combatFamily: 'mystic',
    visual: universalHumanVisual('Jog_Fwd_Loop'),
  },
};

export const isCanonicalBaseClass = (value: unknown): value is BaseClass =>
  typeof value === 'string' && BASE_CLASSES.includes(value as BaseClass);

export const getBaseClassDefinition = (baseClass: BaseClass): CharacterClassDefinition =>
  CHARACTER_CLASS_DEFINITIONS[baseClass];

export const getBaseClassCreationLabel = (baseClass: BaseClass | null): string =>
  baseClass ? getBaseClassDefinition(baseClass).creationLabel : '';

export const getBaseClassCombatFamily = (baseClass: BaseClass): ClassCombatFamily =>
  getBaseClassDefinition(baseClass).combatFamily;
