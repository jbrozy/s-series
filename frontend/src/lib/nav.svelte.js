// Shared state between the project page and the application frame:
// the side navigation shows the sections of the open project.
export const projectNav = $state({
  project: null,   // { id, name, myRole }
  snapshotId: '',  // currently selected upload
  counts: {},      // { findings, images }
  release: ''      // approved | rejected | ''
});

export const ui = $state({
  navOpen: false,      // mobile drawer
  paletteOpen: false,  // Strg+K
  helpOpen: false,     // context help panel
  shortcutsOpen: false // "?" overview
});

export const projectSections = [
  ['overview', 'Übersicht', 'u'],
  ['findings', 'Befunde', 'b'],
  ['release', 'Freigabe', 'f'],
  ['images', 'Bilder', 'i'],
  ['rules', 'Business Rules', 'r'],
  ['knowledge', 'Wissensbasis', 'w'],
  ['training', 'Trainingsdaten', 't'],
  ['members', 'Mitglieder', 'm'],
  ['audit', 'Protokoll', 'l']
];
