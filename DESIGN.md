# S-Check: Gestaltungsrichtung

Grundlage: der Einsatzzweck (Technische Redakteure prüfen S1000D-/S-Series-Exporte vor der Auslieferung) und die UI-Regeln des Projekteigners (keine KI-Klischees, bewusste Typografie, domänenspezifische Palette, Tokens vor Komponenten, Prüfung per Playwright auf Desktop und Mobil).

**Design Read:** interne Fachanwendung für Technische Redakteure, die Exporte auf inhaltliche Fehler prüfen, in der Bildsprache eines technischen Handbuchs und Prüfprotokolls. Dial: ENERGY 1 / RHYTHM 2 / MOTION 1.

- ENERGY 1: Arbeitswerkzeug, das stundenlang offen ist. Der Inhalt (Befunde, DMCs, Bilder) ist laut, die Oberfläche leise.
- RHYTHM 2: gleichmäßiges Raster, aber jede Seite bricht einmal bewusst aus (Kennzahlen-Leiste in der Übersicht, Register bei den Projekten, Protokollblatt bei den Befunden).
- MOTION 1: nur Hover-, Fokus- und Fortschrittszustände. Keine Einblend-Animationen.

## Identitätsmotiv

Zwei Elemente aus gedruckten technischen Handbüchern, überall gleich eingesetzt:

1. **Kapitellinie:** eine 3 px starke Linie in Tinte über jedem Seitenabschnitt (`.ruled`). Abschnitte werden durch Linien gegliedert, nicht durch Kästen.
2. **Änderungsbalken:** In S1000D-Ausgaben markiert ein senkrechter Balken am Rand geänderte Stellen. S-Check nutzt denselben Balken am linken Rand für Zeilen, die Aufmerksamkeit brauchen (Befund nach Schweregrad, Projekt nach Prüfstatus, aktueller Upload). Er steht nur, wo er einen Zustand trägt, und immer zusätzlich zu einem Textlabel.

## Entscheidungen und Gründe

| Entscheidung | Grund |
|---|---|
| Hintergrund Papier `#f4f2ec`, Blätter `#fffefb` | Der Gegenstand ist Dokumentation, die gedruckt oder als IETP auf hellem Grund gelesen wird. Bilder und Anonymisierungen werden auf demselben hellen Grund beurteilt. |
| Nur helles Thema | Farbtreue beim Prüfen von Grafiken (Hautton-Erkennung, Silhouetten) braucht einen neutralen, hellen Grund wie im Zielmedium. Ein dunkles Thema würde die Beurteilung verfälschen. |
| Tinte `#15181c`, Grafit-Kopfzeile | Höchster Kontrast für lange Arbeitssitzungen (15,9:1 auf Papier). |
| Akzent Tintenblau `#0f4a75` | Eine Akzentfarbe für Primäraktion, Links und aktiven Reiter. Blau wie Korrekturtinte, nicht wie Neon. 9,2:1 auf Blatt. |
| Signalorange `#c25400` nur für Fokusring und aktive Hauptnavigation | Muss auf hellen und dunklen Flächen sichtbar sein (4,1:1 bzw. 3,9:1, über 3:1 für Nicht-Text). |
| Schweregrade Rot/Bernstein/Schiefer | Entsprechen WARNING/CAUTION/NOTE in S1000D. Redakteure kennen die Zuordnung. Immer mit Textlabel, nie nur Farbe. |
| Schrift IBM Plex Sans | Für Ingenieursdokumentation entworfen, eindeutige Ziffern, gute Lesbarkeit in dichten Tabellen. |
| IBM Plex Sans Condensed für Überschriften und Reiter | Deutsche Komposita („Publikationsmodule“, „Trainingsdaten“) passen in Reiter und Tabellenköpfe, ohne umzubrechen. |
| IBM Plex Mono nur für Kennungen | DMC, ICN, Dateipfade, XPath und Zeilennummern: Zeichen wie 0/O und 1/l müssen eindeutig sein. Nicht für Daten, Zählungen oder Dekoration. |
| Keine Großbuchstaben-Labels | Labels in normaler Schreibweise, halbfett. Großbuchstaben bleiben den Hinweisen im Dokumentinhalt (WARNUNG, VORSICHT) vorbehalten. |
| Radien 2 px (Felder, Knöpfe, Blätter) und 4 px (Dialoge) | Kanten wie bei Formularen und Datenblättern, keine Kapselform. |
| Schatten nur für Menü, Dialog, Toast | Nur was über der Seite schwebt, wird angehoben (`--e-2`, `--e-3`). Alles andere liegt flach. |
| Abstände im 8-px-Raster (`--s-1` 4 px bis `--s-8` 64 px) | Abschnitte untereinander `--s-6`, Inhalte in Abschnitten `--s-3`/`--s-4`: Weißraum trennt Ebenen. |
| Knoten-Farben im Wissensgraph (7 Farben) | Kategoriale Datenvisualisierung, je Knotentyp eine Farbe, in der Legende mit Text benannt. Gilt nur im Graphen. |
| Schachbrettmuster hinter Bildern | Zeigt transparente Bereiche in Grafiken an, funktional, keine Dekoration. |
| Kein Logo | Es gibt kein freigegebenes Logo. Die Kopfzeile zeigt den Produktnamen als Wortmarke, das Favicon ist ein Platzhalter mit dem Buchstaben S. |

## Tokens

Stehen am Anfang von `frontend/src/app.css`: Abstände, Radien, Elevation, Flächen, Linien, Farben, Schriftgrößen. Komponenten verwenden nur diese Tokens.

## Mobil

Unter 760 px: alle Bedienelemente mindestens 44 px hoch, Reiter umbrechen statt abgeschnitten zu werden, breite Tabellen scrollen innerhalb ihres Blatts, Seitenspalten stapeln sich.
