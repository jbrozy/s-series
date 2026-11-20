# S-Check Styleguide

Grundlage: DIN EN ISO 9241-110:2020 (Interaktionsprinzipien), -112:2017 (Grundsätze der Informationsdarstellung), -125:2017 (visuelle Informationsdarstellung), -143:2012 (Formulardialoge), WCAG 2.2 Stufe AA (über EN 301 549 / BITV 2.0) und Arbeitsstättenverordnung Anhang 6.5 (Software an Kenntnisse der Beschäftigten anpassbar, Angaben zu Dialogabläufen).

S-Check ist ein Arbeitswerkzeug für Technische Redakteure. Die Oberfläche soll aussehen und sich verhalten wie eine sorgfältige Fachanwendung: ruhig, vorhersehbar, dicht genug für lange Listen. Auffallen sollen nur der Inhalt und Zustände, die eine Handlung verlangen.

**Design Read:** Fachanwendung (Prüfwerkzeug) für Technische Redakteure und QS, klassischer Aufbau mit Seitenleiste. Dial: ENERGY 1 / RHYTHM 1 / MOTION 1. Einheitlichkeit ist hier Absicht (ISO 9241-110, Erwartungskonformität), nicht Zufall.

**Identitätsmotiv:** Der Seitenkopf ist auf jeder Seite gleich aufgebaut: Pfad, Titel, Statuszeile, Aktionen rechts. Wer eine Seite kennt, findet sich auf allen zurecht.

## 1 Interaktionsprinzipien (ISO 9241-110) und ihre Umsetzung

| Prinzip | Umsetzung in S-Check |
|---|---|
| Aufgabenangemessenheit | Startpunkt ist die Arbeit: offene Befunde, Freigabestatus, „Mir zugewiesen“. Sammelaktionen für wiederkehrende Bearbeitung. Filter bleiben je Projekt gespeichert. |
| Selbstbeschreibungsfähigkeit | Jede Seite hat Titel, Pfad und eine Statuszeile. Kontexthilfe (F1 bzw. Schaltfläche „Hilfe“) erklärt die aktuelle Seite. Leere Zustände sagen, warum sie leer sind und was zu tun ist. |
| Erwartungskonformität | Navigation links, Suche oben, Konto oben rechts. Gleiche Begriffe überall (Upload, Analyse, Befund, Freigabe). Gleiche Aktion steht immer an derselben Stelle. |
| Erlernbarkeit | Handbuch und Glossar in der Anwendung, Tastenkürzel-Übersicht unter „?“, Beschriftungen statt reiner Symbole. |
| Steuerbarkeit | Jeder Dialog lässt sich mit Abbrechen oder Escape verlassen. Sortierung, Filter, Dichte, Design und Navigation sind wählbar. Lange Vorgänge zeigen Fortschritt. |
| Robustheit gegen Benutzungsfehler | Löschen und Ablehnen fragen nach. Verwerfen und Freigabe trotz Verstoß verlangen eine Begründung. Fehlermeldungen nennen Ursache und nächsten Schritt und stehen am Feld. |
| Benutzerbindung | Keine Ablenkung, keine Animationen. Rückmeldung nach jeder Aktion in Klartext („3 Befunde bestätigt“). |

## 2 Informationsdarstellung (ISO 9241-112)

- **Entdeckbarkeit:** Alle Funktionen sind über die Seitenleiste oder sichtbare Schaltflächen erreichbar; Tastenkürzel sind nur Abkürzungen, nie der einzige Weg.
- **Ablenkungsfreiheit:** Keine Verläufe, Schatten nur für schwebende Elemente (Menü, Dialog, Meldung), keine Dekoration.
- **Unterscheidbarkeit:** Gruppen werden durch Flächen mit Rahmen und Kopfzeile getrennt (Panel), nicht durch Farbe allein.
- **Eindeutige Interpretierbarkeit:** Zahlen mit Einheit oder Bezeichnung, Datum im deutschen Format, Zustände ausgeschrieben.
- **Kompaktheit:** Zwei Dichten (normal, kompakt). Tabellen zeigen ein Objekt pro Zeile.
- **Konsistenz:** Ein Satz Komponenten, ein Satz Tokens, siehe unten.

## 3 Visuelle Darstellung (ISO 9241-125)

### Schrift

- Systemschrift: Segoe UI (Windows), sonst Noto Sans bzw. Liberation Sans. Grund: Sie ist für Bildschirmdarstellung optimiert und den Beschäftigten aus Windows vertraut (Erwartungskonformität); es wird keine Webschrift geladen, die Anwendung funktioniert offline identisch.
- Festbreitenschrift (Cascadia Mono, Consolas) nur für Kennungen: DMC, ICN, XPath, Dateipfade, Zeilennummern. Dort müssen 0/O und 1/l eindeutig sein.
- Grundgröße 14 px, Sekundärtext 13 px, nie unter 12 px. Überschriften 24/18/16 px, Gewicht 600. Keine Versalien-Auszeichnung, keine gesperrte Schrift.
- Zeilenhöhe 1,5 für Fließtext, Absatzbreite höchstens etwa 80 Zeichen.
- Vergrößerung über Browser-Zoom bis 200 % ohne Informationsverlust (WCAG 1.4.4, 1.4.10).

### Farbe

Neutrale Flächen, eine Aktionsfarbe, Signalfarben nur für Zustände. Farbe codiert nie allein: jeder Zustand hat ein Textlabel (ISO 9241-125 Farbcodierung, WCAG 1.4.1).

| Rolle | Hell | Dunkel | Kontrast (geprüft) |
|---|---|---|---|
| Hintergrund Anwendung | `#f3f4f6` | `#14171c` | |
| Fläche (Panel, Tabelle) | `#ffffff` | `#1c2027` | |
| Text | `#1d2330` | `#e7e9ed` | 15,7 : 1 / 13,4 : 1 |
| Sekundärtext | `#5a6372` | `#a3abb8` | 6,1 : 1 / 7,1 : 1 |
| Aktionsfarbe (Link, Primärknopf, Fokus) | `#0b5cad` | `#79b0f0` (Knopf `#2f6fbf`) | 6,7 : 1 / 7,2 : 1 |
| Feldrahmen | `#7a8496` | `#6f7a8c` | 3,4 : 1 min. / 3,8 : 1 |
| Fehler | `#b42318` auf `#fdecea` | `#ff9b8f` auf `#3a1f1f` | 5,8 : 1 / 7,4 : 1 |
| Warnung | `#8a4b00` auf `#fff4e5` | `#f0b45a` auf `#352a14` | 6,3 : 1 / 7,6 : 1 |
| In Ordnung | `#1e6b35` auf `#e7f4ec` | `#7fd39b` auf `#173022` | 5,8 : 1 / 7,9 : 1 |
| Hinweis | `#2a5a8a` auf `#e9f1f9` | `#93bdf0` auf `#182a3d` | 6,3 : 1 / 7,5 : 1 |

Rot, Gelb/Orange und Grün folgen der üblichen Bedeutung (Fehler, Warnung, in Ordnung). Blau ist ausschließlich Aktionsfarbe und für Hinweise.

### Raster, Formen, Ebenen

- Abstände im 8-px-Raster (4, 8, 12, 16, 24, 32, 48).
- Radius 4 px für Felder, Knöpfe und Panels, 6 px für Dialoge. Keine Kapselformen außer für Zähler.
- Ebenen: Anwendung → Panel (Rahmen, kein Schatten) → schwebend (Menü, Dialog, Meldung mit Schatten).

## 4 Formulare (ISO 9241-143)

- Beschriftung über dem Feld, linksbündig, immer sichtbar; Platzhalter nur als Beispiel, nie als Beschriftung.
- Pflichtfelder sind als solche beschriftet, optionale Felder mit „(optional)“.
- Prüfung beim Absenden; Fehlertext direkt am Feld, verbunden über `aria-describedby`, Fokus auf das erste fehlerhafte Feld.
- Primäraktion rechts unten, Abbrechen links daneben. Eine Primäraktion pro Ansicht.
- Zerstörende Aktionen in Rot mit Rückfrage, die das Objekt nennt.

## 5 Barrierefreiheit (WCAG 2.2 AA)

- Kontraste wie oben; Fokusrahmen 2 px in Aktionsfarbe mit 2 px Abstand, nie verdeckt (2.4.11).
- Zielgrößen mindestens 24 × 24 px (2.5.8), auf Touch-Geräten 44 px.
- Alles per Tastatur bedienbar, Reihenfolge = Lesereihenfolge, „Zum Inhalt springen“.
- Einzeltasten-Kürzel lassen sich in den Einstellungen abschalten (2.1.4).
- Statusmeldungen über `role="status"`, Fehler über `role="alert"`.
- Bewegung nur, wenn „Bewegung reduzieren“ nicht gesetzt ist; es gibt ohnehin keine Animationen außer Fortschrittsbalken.

## 6 Aufbau

- **Kopfzeile:** Produktname, Navigation ein-/ausblenden, globale Suche (Strg+K), Hilfe, Konto.
- **Seitenleiste:** Bereiche „Projekte“, „Administration“ (nur Admins), „Einstellungen“; in einem Projekt zusätzlich alle Projektbereiche mit Zählern. Ausblendbar; auf schmalen Bildschirmen als Menü.
- **Seitenkopf:** Pfad, Titel, Statuszeile, Aktionen rechts.
- **Inhalt:** Panels mit Kopfzeile (Titel links, Aktionen rechts). Tabellen: Text linksbündig, Zahlen rechtsbündig, sortierbare Spalten mit Pfeil und `aria-sort`.

## 7 Tokens

Stehen am Anfang von `frontend/src/app.css` (`:root` für hell, `[data-theme="dark"]` für dunkel, `[data-density="compact"]` für die kompakte Dichte). Komponenten verwenden ausschließlich diese Tokens.
