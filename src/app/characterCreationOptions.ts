import type { BaseClass, CharacterCatalogRace, CharacterCatalogResponse, CharacterRace, CharacterSex } from '../online/contracts';

export interface CharacterCreationOptions {
  selectedRace: CharacterRace | null;
  raceOptions: CharacterCatalogRace[];
  selectedBaseClass: BaseClass | null;
  baseClassOptions: BaseClass[];
  selectedSex: CharacterSex | null;
  sexOptions: CharacterSex[];
  selectedHairStyle: number | null;
  selectedHairColor: string | null;
  selectedSkinType: number | null;
  hairStyleOptions: number[];
  skinTypeOptions: number[];
}

export const normalizeCanonicalHairColor = (value: string | null | undefined): string | null => {
  const normalized = value?.trim().toLowerCase() ?? '';
  return /^#[0-9a-f]{6}$/.test(normalized) ? normalized : null;
};

const raceSortOrder: CharacterRace[] = ['Human', 'Elf', 'Dark Elf', 'Orc', 'Dwarf'];

const compareRace = (left: CharacterCatalogRace, right: CharacterCatalogRace): number =>
  raceSortOrder.indexOf(left.race) - raceSortOrder.indexOf(right.race);

export const resolveCharacterCreationOptions = (
  catalog: CharacterCatalogResponse | null,
  selectedRace: CharacterRace | null,
  selectedBaseClass: BaseClass | null = null,
  selectedSex: CharacterSex | null = null,
  selectedHairStyle: number | null = null,
  selectedSkinType: number | null = null,
  selectedHairColor: string | null = null,
): CharacterCreationOptions => {
  const raceOptions = [...(catalog?.races ?? [])].filter((race) => race.enabled).sort(compareRace);
  const activeRace =
    selectedRace && raceOptions.find((race) => race.race === selectedRace)
      ? selectedRace
      : raceOptions[0]?.race ?? null;
  const selectedRaceEntry = activeRace ? raceOptions.find((race) => race.race === activeRace) ?? null : null;
  const baseClassOptions = [...(selectedRaceEntry?.base_classes ?? [])];
  const sexOptions = [...(selectedRaceEntry?.sex_options ?? [])];
  const hairStyleOptions = [...(selectedRaceEntry?.appearance_options?.hair_styles ?? [])];
  const skinTypeOptions = [...(selectedRaceEntry?.appearance_options?.skin_types ?? [])];
  const activeBaseClass =
    selectedBaseClass && baseClassOptions.includes(selectedBaseClass)
      ? selectedBaseClass
      : baseClassOptions[0] ?? null;
  const activeSex = selectedSex && sexOptions.includes(selectedSex) ? selectedSex : sexOptions[0] ?? null;
  const activeHairStyle =
    typeof selectedHairStyle === 'number' && hairStyleOptions.includes(selectedHairStyle)
      ? selectedHairStyle
      : hairStyleOptions[0] ?? null;
  const activeHairColor =
    normalizeCanonicalHairColor(selectedHairColor) ??
    normalizeCanonicalHairColor(selectedRaceEntry?.appearance_options?.hair_color_default);
  const activeSkinType =
    typeof selectedSkinType === 'number' && skinTypeOptions.includes(selectedSkinType)
      ? selectedSkinType
      : skinTypeOptions[0] ?? null;

  return {
    selectedRace: activeRace,
    raceOptions,
    selectedBaseClass: activeBaseClass,
    baseClassOptions,
    selectedSex: activeSex,
    sexOptions,
    selectedHairStyle: activeHairStyle,
    selectedHairColor: activeHairColor,
    selectedSkinType: activeSkinType,
    hairStyleOptions,
    skinTypeOptions,
  };
};
