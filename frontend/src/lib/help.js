// Context help and handbook content. One entry per page; the help panel
// shows the entry of the current page, the handbook shows all of them.

export const helpPages = {
  projects: {
    title: 'Projekte',
    purpose: 'Liste aller Projekte, auf die Sie Zugriff haben. Jedes Projekt hat eigene Uploads, Business Rules, Freigabekriterien und Mitglieder.',
    steps: [
      'Spaltenköpfe anklicken, um die Liste zu sortieren.',
      'Mit „Neues Projekt“ ein Projekt anlegen; Sie werden dessen Eigentümer.',
      'Die Spalte „Prüfstatus“ zeigt den Stand des neuesten Uploads, „Freigabe“ die letzte Entscheidung dazu.'
    ]
  },
  overview: {
    title: 'Projektübersicht',
    purpose: 'Stand des gewählten Uploads: Inhalt, Ergebnis der letzten Analyse, Freigabestatus und die enthaltenen Dateien.',
    steps: [
      'Neuen Export rechts hochladen (ZIP, Ordner oder einzelne Dateien; XML/SGML oder HTML-Export). Er wird eingelesen und direkt geprüft.',
      'Über „Upload / Stand“ oben zu einem früheren Upload wechseln.',
      'Einen DMC in der Dateiliste anklicken, um den Quelltext zu sehen.',
      '„Neue Analyse“ prüft erneut, z. B. nach geänderten Business Rules.'
    ]
  },
  findings: {
    title: 'Befunde',
    purpose: 'Alle inhaltlichen Auffälligkeiten der letzten Analyse. Jeder Befund nennt Fundstelle, Meldung und meist einen Vorschlag.',
    steps: [
      'Filtern nach Schwere, Kategorie, Status, Zuständigen oder Vergleich mit dem vorherigen Upload. Filter und Sortierung bleiben für dieses Projekt gespeichert.',
      'Bestätigen: Befund ist richtig und wird korrigiert. Behoben: im Export bereits korrigiert. Verwerfen: kein Fehler, mit Begründung; bleibt auch in späteren Analysen verworfen.',
      'Mehrere Befunde über die Kästchen auswählen und gemeinsam zuweisen oder bearbeiten.',
      '„Verlauf“ zeigt Kommentare und Änderungen; sie bleiben am Befund, solange er in späteren Uploads wieder auftritt.'
    ]
  },
  release: {
    title: 'Freigabe',
    purpose: 'Entscheidung, ob ein Upload ausgeliefert werden darf, gemessen an den Freigabekriterien des Projekts.',
    steps: [
      'Eigentümer legen die Kriterien rechts fest: Höchstzahl offener Fehler, Warnungen und Hinweise, Kategorien ohne offene Befunde, KI-Review erforderlich.',
      'Gezählt werden offene und bestätigte Befunde; begründet verworfene und behobene zählen nicht.',
      'Freigeben trotz nicht erfüllter Kriterien und jede Ablehnung brauchen eine Begründung. Nach einer neuen Analyse ist neu zu entscheiden.',
      '„Prüfbericht öffnen“ erzeugt den druckbaren Nachweis.'
    ]
  },
  images: {
    title: 'Bilder',
    purpose: 'Prüfung der Grafiken auf menschliche Körperteile, die laut Standard nicht abgebildet sein dürfen.',
    steps: [
      'Ein Bild öffnen, Markierungen prüfen und bei Bedarf korrigieren.',
      'Anonymisieren: Hände werden als neutrale Silhouette dargestellt, Gesichter verpixelt.',
      'Bilder ohne Körperteile als unbedenklich freigeben. Der Export ersetzt anonymisierte Grafiken.'
    ]
  },
  rules: {
    title: 'Business Rules',
    purpose: 'Projektspezifische Regeln, die bei jeder Analyse geprüft werden: verbotene Begriffe, XPath-Bedingungen, Satzlänge, Schreibweise, Wörterbuch.',
    steps: [
      '„Neue Regel“ wählen und ausfüllen; die Regel gilt ab der nächsten Analyse.',
      '„BREX importieren“ übernimmt Strukturregeln aus dem BREX-Datenmodul des Uploads.',
      '„Regeln exportieren“ und „Regeln übernehmen“ übertragen Regelsätze zwischen Projekten.'
    ]
  },
  knowledge: {
    title: 'Wissensbasis',
    purpose: 'Wissensgraph der Projektstruktur und Volltextsuche über alle Datenmodule des Uploads.',
    steps: [
      'Im Graphen einen Knoten anklicken, um Verweise, Grafiken und Teile zu sehen.',
      'Unter „Suche & Fragen“ nach Begriffen suchen oder eine Frage an das Modell stellen (falls eingerichtet).'
    ]
  },
  training: {
    title: 'Trainingsdaten',
    purpose: 'Erzeugt Beispieldaten aus korrekten Datenmodulen mit eingebauten Fehlern, um ein eigenes Prüfmodell anzulernen.',
    steps: ['Fehlerarten und Anzahl wählen, Statistik prüfen, JSONL-Datei herunterladen.']
  },
  members: {
    title: 'Mitglieder',
    purpose: 'Wer das Projekt sehen (Leser), bearbeiten (Redakteur) oder verwalten (Eigentümer) darf.',
    steps: ['Personen über Name oder E-Mail suchen und mit Rolle hinzufügen.', 'Rollen in der Tabelle ändern; ein Projekt behält immer mindestens einen Eigentümer.']
  },
  audit: {
    title: 'Protokoll',
    purpose: 'Nachweis aller Änderungen im Projekt: wer hat wann was geändert.',
    steps: ['Nach Bereich, Zeitraum oder Text filtern.', '„Als CSV exportieren“ liefert die gefilterten Einträge für Audits.']
  },
  report: {
    title: 'Prüfbericht',
    purpose: 'Druckbarer Nachweis eines Uploads mit Ergebnis, Kriterien, Befunden und Entscheidungsverlauf.',
    steps: ['„Drucken oder als PDF speichern“ öffnet den Druckdialog des Browsers; dort „Als PDF speichern“ wählen.']
  },
  admin: {
    title: 'Administration',
    purpose: 'Modell-Schnittstellen, Benutzerkonten, Systemstatus und das Protokoll aller Projekte.',
    steps: ['Modelle: OpenAI-kompatible Endpunkte für KI-Review, Embeddings und Bildprüfung eintragen und testen.', 'Benutzer: Konten anlegen, sperren, Passwörter zurücksetzen.']
  },
  account: {
    title: 'Konto',
    purpose: 'Name, Passwort und API-Tokens für Skripte und Pipelines.',
    steps: ['Tokens einzeln pro Pipeline anlegen, damit sie sich einzeln widerrufen lassen.', 'scheck-ci herunterladen, um Exporte in einer Pipeline prüfen zu lassen.']
  },
  settings: {
    title: 'Einstellungen',
    purpose: 'Persönliche Darstellung und Bedienung. Die Einstellungen gelten für Ihr Konto auf allen Arbeitsplätzen.',
    steps: ['Design (hell, dunkel oder wie im Betriebssystem), Tabellendichte, Startseite und Tastenkürzel festlegen.', 'Schrift vergrößern: Browser-Zoom mit Strg und + (bis 200 %).']
  }
};

export const shortcuts = [
  { keys: ['Strg', 'K'], text: 'Suche öffnen (Projekte, Datenmodule, Befunde, Regeln, Seiten)', single: false },
  { keys: ['F1'], text: 'Hilfe zur aktuellen Seite', single: false },
  { keys: ['Esc'], text: 'Dialog, Menü oder Hilfe schließen', single: false },
  { keys: ['/'], text: 'Suche öffnen', single: true },
  { keys: ['?'], text: 'Diese Übersicht', single: true },
  { keys: ['g', 'p'], text: 'Projekte', single: true },
  { keys: ['g', 'e'], text: 'Einstellungen', single: true },
  { keys: ['g', 'u'], text: 'Projekt: Übersicht', single: true },
  { keys: ['g', 'b'], text: 'Projekt: Befunde', single: true },
  { keys: ['g', 'f'], text: 'Projekt: Freigabe', single: true },
  { keys: ['g', 'r'], text: 'Projekt: Business Rules', single: true },
  { keys: ['j'], text: 'Befunde: nächster Befund', single: true },
  { keys: ['k'], text: 'Befunde: vorheriger Befund', single: true },
  { keys: ['x'], text: 'Befunde: aktuellen Befund auswählen', single: true }
];

export const glossary = [
  ['Datenmodul (DM)', 'Kleinste eigenständige Informationseinheit in S1000D, identifiziert durch den Datenmodulcode.'],
  ['DMC', 'Datenmodulcode, z. B. DMC-S1KD-A-29-10-00-00A-520A-A: Modell, System, Baugruppe, Infocode (520 = Ausbau).'],
  ['Publikationsmodul (PM)', 'Legt fest, welche Datenmodule in welcher Reihenfolge eine Publikation bilden.'],
  ['ICN', 'Kennung einer Grafik oder eines Medienobjekts, auf das Datenmodule verweisen.'],
  ['BREX', 'Business Rules Exchange: Datenmodul mit projektspezifischen Strukturregeln, aus dem S-Check Regeln übernehmen kann.'],
  ['Upload', 'Ein hochgeladener Projektexport zu einem Zeitpunkt. Jeder Upload wird für sich analysiert und freigegeben.'],
  ['Analyse', 'Prüflauf über einen Upload mit den gewählten Prüfungen und den Business Rules des Projekts.'],
  ['Befund', 'Eine gemeldete Auffälligkeit mit Fundstelle. Status: offen, bestätigt, behoben, verworfen.'],
  ['Freigabekriterien', 'Grenzwerte des Projekts, die ein Upload für die Freigabe erfüllen muss.'],
  ['Prüfbericht', 'Druckbarer Nachweis über Ergebnis, Befunde und Freigabe eines Uploads.'],
  ['API-Token', 'Zugangsschlüssel für Skripte und Pipelines mit den Projektrechten des Kontos, das ihn erstellt hat.'],
  ['S-Check Desktop', 'Installierbare Windows-Anwendung mit derselben Oberfläche. Sie verbindet sich bei der Anmeldung mit einem S-Check-Server; Daten liegen nur dort. Server wechseln: Benutzermenü → Server wechseln.']
];

export const standards = [
  'DIN EN ISO 9241-110:2020 Interaktionsprinzipien',
  'DIN EN ISO 9241-112 und -125 Informationsdarstellung',
  'DIN EN ISO 9241-143 Formulardialoge',
  'WCAG 2.2 Stufe AA (EN 301 549, BITV 2.0)',
  'Arbeitsstättenverordnung Anhang 6.5'
];

/** Help key for the current route. */
export function helpKey(route) {
  const [a, , c] = route.parts;
  if (!a) return 'projects';
  if (a === 'p') return helpPages[c || 'overview'] ? (c || 'overview') : 'overview';
  if (a === 'einstellungen') return 'settings';
  if (a === 'admin') return 'admin';
  if (a === 'account') return 'account';
  return 'projects';
}
