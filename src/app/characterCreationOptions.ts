import type { BaseClass, CharacterCatalogRace, CharacterCatalogResponse, CharacterRace, CharacterSex } from '../online/contracts';

export interface CharacterCreationOptions {
  selectedRace: CharacterRace | null;
  raceOptions: CharacterCatalogRace[];
  selectedBaseClass: BaseClass | null;
  baseClassOptions: BaseClass[];
  selectedSex: CharacterSex | null;
  sexOptions: CharacterSex[];
  selectedHairStyle: number | null;
  selectedHairColor: number | null;
  selectedFace: number | null;
  hairStyleOptions: number[];
  hairColorOptions: number[];
  faceOptions: number[];
}

const raceSortOrder: CharacterRace[] = ['Human', 'Elf', 'Dark Elf', 'Orc', 'Dwarf'];

const compareRace = (left: CharacterCatalogRace, right: CharacterCatalogRace): number =>
  raceSortOrder.indexOf(left.race) - raceSortOrder.indexOf(right.race);

export const resolveCharacterCreationOptions = (
  catalog: CharacterCatalogResponse | null,
  selectedRace: CharacterRace | null,
  selectedBaseClass: BaseClass | null = null,
  selectedSex: CharacterSex | null = null,
  selectedHairStyle: number | null = null,
  selectedHairColor: number | null = null,
  selectedFace: number | null = null,
): CharacterCreationOptions => {
  const raceOptions = [...(catalog?.races ?? [])].filter((race) => race.enabled).sort(compareRace);
  const activeRace = selectedRace && raceOptions.find((race) => race.race === selectedRace) ? selectedRace : null;
  const selectedRaceEntry = activeRace ? raceOptions.find((race) => race.race === activeRace) ?? null : null;
  const baseClassOptions = [...(selectedRaceEntry?.base_classes ?? [])];
  const sexOptions = [...(selectedRaceEntry?.sex_options ?? [])];
  const hairStyleOptions = [...(selectedRaceEntry?.appearance_options?.hair_styles ?? [])];
  const hairColorOptions = [...(selectedRaceEntry?.appearance_options?.hair_colors ?? [])];
  const faceOptions = [...(selectedRaceEntry?.appearance_options?.faces ?? [])];
  const activeBaseClass =
    selectedBaseClass && baseClassOptions.includes(selectedBaseClass) ? selectedBaseClass : null;
  const activeSex = selectedSex && sexOptions.includes(selectedSex) ? selectedSex : null;
  const activeHairStyle =
    typeof selectedHairStyle === 'number' && hairStyleOptions.includes(selectedHairStyle) ? selectedHairStyle : null;
  const activeHairColor =
    typeof selectedHairColor === 'number' && hairColorOptions.includes(selectedHairColor) ? selectedHairColor : null;
  const activeFace = typeof selectedFace === 'number' && faceOptions.includes(selectedFace) ? selectedFace : null;

  return {
    selectedRace: activeRace,
    raceOptions,
    selectedBaseClass: activeBaseClass,
    baseClassOptions,
    selectedSex: activeSex,
    sexOptions,
    selectedHairStyle: activeHairStyle,
    selectedHairColor: activeHairColor,
    selectedFace: activeFace,
    hairStyleOptions,
    hairColorOptions,
    faceOptions,
  };
};
