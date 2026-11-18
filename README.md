# S-Check: semantische Prüfung für S-Series-Dokumentation

## Starten

```bash
cp .env.example .env      # Passwörter anpassen (optional)
docker compose up -d --build
```

Danach `http://localhost:8080` öffnen. Der erste registrierte Benutzer wird Administrator.
Zum Ausprobieren `testdata/beispiel-export.zip` in einem neuen Projekt hochladen (enthält absichtlich Fehler).
`testdata/graph-demo.zip` (60 Datenmodule, ein Publikationsmodul) zeigt den Wissensgraphen.

Lokale Modelle ohne Internet: `docker compose --profile ollama up -d`, dann unter
*Administration → Modelle* `http://ollama:11434/v1` eintragen.

## Benutzer und Rechte

- Der erste registrierte Benutzer wird Administrator. Unter *Administration → Benutzer* lassen sich Konten anlegen (mit Startpasswort),
  bearbeiten, sperren, Passwörter zurücksetzen und Konten löschen (Projekte gehen dann an den löschenden Admin). Die Selbstregistrierung
  ist dort abschaltbar.
- Unter *Projekt → Mitglieder* fügen Eigentümer Personen per Suche hinzu (Leser, Redakteur, Eigentümer); Administratoren können dort auch
  direkt ein neues Konto anlegen. Ein Projekt behält immer mindestens einen Eigentümer.
- Wer ein Startpasswort erhält, muss beim ersten Anmelden ein eigenes festlegen. Passwortwechsel, Sperren und Zurücksetzen beenden
  bestehende Anmeldungen.

## Freigabe, Nachweise und Zusammenarbeit

- **Befund-Workflow:** Befunde lassen sich Mitgliedern zuweisen und kommentieren (*Verlauf*). Zuweisung und Kommentare hängen am
  Fingerabdruck des Befunds und bleiben über spätere Uploads erhalten, solange der Befund wieder auftritt. Verwerfen braucht eine Begründung.
- **Upload-Vergleich:** In *Befunde* zeigt jeder Upload, was gegenüber dem vorherigen neu, unverändert oder nicht mehr vorhanden ist.
- **Freigabe:** Eigentümer legen unter *Freigabe* Kriterien fest (höchstens n offene Fehler/Warnungen/Hinweise, Kategorien ohne offene
  Befunde, KI-Review erforderlich). Ohne Festlegung gilt: keine offenen Fehler. Ein Upload wird freigegeben oder abgelehnt; eine Freigabe
  trotz nicht erfüllter Kriterien und jede Ablehnung braucht eine Begründung. Nach einer neuen Analyse ist die Entscheidung neu zu treffen.
- **Prüfbericht:** *Freigabe → Prüfbericht öffnen* erzeugt einen druckbaren Bericht (Ergebnis, geprüfter Stand, Befunde nach Kategorie,
  offene und begründet verworfene Befunde, Entscheidungsverlauf). PDF über den Druckdialog des Browsers, ohne Internet.
- **Protokoll:** Jede Änderung an Befunden, Regeln, Kriterien, Freigaben, Uploads, Mitgliedern, Bildern, Konten, Einstellungen und Tokens
  wird mit Person, Zeitpunkt, IP und Details protokolliert (*Projekt → Protokoll* für Redakteure und Eigentümer, *Administration →
  Protokoll* für alles). Filter nach Bereich, Zeitraum und Text; Export als CSV. Einträge bleiben erhalten, wenn Projekte oder Konten gelöscht werden.
- **Regelsets:** *Business Rules → Regeln exportieren* speichert alle Regeln als Datei; *Regeln übernehmen* holt sie aus einer Datei oder
  einem anderen Projekt (gleichnamige Regeln werden übersprungen).

## Automatisierung (CSDB-Export, CI)

Unter *Konto → API-Tokens* erzeugt jeder Benutzer Tokens (nur einmal im Klartext angezeigt, gespeichert wird eine Prüfsumme). Ein Token hat
die Projektrechte seines Benutzers, kann aber keine Konten, Tokens oder Administration erreichen. Aktionen per Token stehen mit dem
Token-Namen im Protokoll.

`scheck-ci` lädt einen Export hoch, wartet auf die Analyse und gibt das Ergebnis der Freigabekriterien als Rückgabewert zurück
(0 erfüllt, 1 nicht erfüllt, 2 Fehler). Das Programm liegt für Linux und Windows im Backend-Image und lässt sich unter *Konto* herunterladen,
also auch ohne Internet auf Build-Agenten.

```bash
export SCHECK_URL=https://s-check.intern SCHECK_TOKEN=sck_... SCHECK_PROJECT=<Projekt-ID>
scheck-ci -label "Issue 004" -csv befunde.csv export.zip      # Datei, ZIP oder Ordner
```

Direkt per HTTP: `Authorization: Bearer <Token>`, z. B. `POST /api/projects/{id}/snapshots` (multipart), `GET /api/snapshots/{id}/release`.

## Kubernetes (Ansible)

Ein Playbook, alle Einstellungen stehen oben in `deploy/ansible/deploy.yml` (Namespace `s1000-helper`):

```bash
ansible-galaxy collection install kubernetes.core community.docker
pip install kubernetes
cd deploy/ansible
ansible-playbook deploy.yml -e image_registry=registry.firma.de/s1000 -e ingress_host=s1000.firma.de
ansible-playbook deploy.yml --skip-tags build        # nur ausrollen
ansible-playbook deploy.yml --tags render            # Manifeste nach rendered/ (kubectl/GitOps)
ansible-playbook deploy.yml --tags teardown -e confirm_delete=s1000-helper   # alles inkl. Daten löschen
```

Passwörter über Umgebungsvariablen (`S1000_POSTGRES_PASSWORD`, `S1000_REDIS_PASSWORD`, `S1000_ADMIN_PASSWORD`,
`S1000_CHAT_API_KEY` …). Fehlen sie, werden zufällige Passwörter erzeugt und unter `deploy/ansible/credentials/` gespeichert.

## Aufbau

| Ordner | Inhalt |
|---|---|
| `backend/` | Go-API + Worker. Abhängigkeiten liegen in `vendor/`, der Build funktioniert offline. |
| `frontend/` | Svelte-5-Oberfläche, ausgeliefert über nginx (Proxy `/api` → Backend). |
| `database/` | Postgres-Image, Konfiguration und SQL-Migrationen (vom Backend beim Start angewendet). |
| `deploy/ansible/` | Kubernetes-Deployment (Namespace `s1000-helper`). |

Persistente Volumes: `pgdata` (Datenbank), `redisdata` (Sessions/Queue), `appdata` (hochgeladene Dateien, anonymisierte Bilder).

## Prüfungen

- **Listen & Duplikate:** doppelte/fast doppelte Listeneinträge, Arbeitsschritte, Betriebsstoffe/Ersatzteile/Hilfsmittel, Tabellenzeilen, Listen mit nur einem Eintrag
- **Überschriften & Schreibung:** leere Titel, Satzzeichen, doppelte Titel (Geschwister, im DM, DM-Titel projektweit), Groß-/Kleinschreib- und Schreibvarianten, korpusbasierte Tippfehler-Erkennung (sprachunabhängig, Projektwörterbuch), uneinheitlicher `infoName` je Infocode und `techName` je SNS
- **Verweise:** `internalRef`/`warningRefs`/`cautionRefs`, `dmRef`, `pmRef`, fehlende und ungenutzte ICN-Grafiken
- **Konsistenz:** widersprüchliche Abkürzungsdefinitionen, unterschiedliche Benennung derselben Teilenummer, leicht abweichende Warn-/Vorsichtshinweise, leere Absätze, mehrfach exportierte DMs
- **Business Rules je Projekt:** XPath (verboten/erforderlich/Werte), unzulässige Begriffe, Titelschreibweise, Satzlänge (STE), Wörterbuch, BREX-Import
- **KI-Review (optional):** beliebiges OpenAI-kompatibles Modell, mit Kontext aus anderen DMs (Volltext + Embeddings)
- **Bilder:** Erkennung von Körperteilen (Vision-LLM, eigener HTTP-Detektor oder Heuristik), Editor zum Korrigieren, Anonymisierung (Hände als Silhouette, Gesichter verpixelt), Export mit ersetzten Grafiken

## HTML-Export laden

Statt des XML-Exports kann auch ein veröffentlichter HTML-Export (IETP-/CSDB-Ausgabe, statisches HTML-Handbuch) hochgeladen werden, als ZIP oder Ordner mit Seiten und Bildern. Jede Seite wird in eine S1000D-ähnliche Struktur übersetzt, damit alle Prüfungen, der Wissensgraph und der KI-Review darauf laufen. Zeilennummern in Befunden beziehen sich auf den HTML-Quelltext.

- **DMC** aus Dateiname, `<meta name="dmc">`, `<title>` oder „DMC: …“ im Text; Ausgabestand und Sprache aus dem Dateinamen (`_001-00_DE-DE`) bzw. `lang`.
- **Inhalt:** `h1` bis `h6` → `levelledPara`, `ul`/`ol` → Listen, bei Prozedur-Infocodes (1xx bis 8xx) `ol` → `proceduralStep`, Tabellen, Abbildungen (`img`-Dateiname = ICN), Warnungen/Vorsicht/Hinweise über CSS-Klasse oder Schlüsselwort. Navigation, Kopf- und Fußzeilen werden ignoriert.
- **Verweise:** `#anker` → `internalRef`, Links auf andere Seiten → `dmRef`; Links ins Leere ergeben `REF_LINK_MISSING`. Eine Start-/Inhaltsseite ohne DMC wird zum Publikationsmodul.
- Enthält ein Export dieselbe DM als XML **und** HTML, zählt nur das XML.
- Grenzen: Was beim Veröffentlichen verloren geht (Applicability, Metadaten, Attribute), kann nicht geprüft werden; BREX-/XPath-Regeln greifen nur auf die übersetzte Struktur. Hochgeladenes HTML wird nie im Browser ausgeführt, sondern nur als Quelltext angezeigt.

## Wissensgraph

Beim Einlesen entsteht pro Upload ein Graph in Postgres (`graph_nodes`, `graph_edges`): Produktgliederung (SNS: System › Subsystem › Baugruppe),
Datenmodule, Publikationsmodule, Grafiken, Teile/Betriebsstoffe und Abkürzungen, verbunden über *gehört zu*, *verweist auf*, *enthält*,
*verwendet Grafik*, *benötigt/listet Teil* und *definiert*. Unter *Wissensbasis → Wissensgraph* lässt er sich als Hierarchie oder Netz
erkunden; Befunde, fehlende Verweisziele, ungenutzte Grafiken und Datenmodule ohne Publikationsmodul werden hervorgehoben.
Der KI-Review nutzt die Graph-Nachbarn (Verweise, gleiche Baugruppe) als Kontext.

Dismissed-Befunde bleiben in späteren Analysen des Projekts verworfen. Befunde lassen sich als CSV exportieren.

## Trainingsdaten

*Projekt → Trainingsdaten* erzeugt JSONL (Chat- oder Alpaca-Format) aus korrekten Datenmodulen, in die typische Fehler eingebaut werden. Prompt und Antwortformat entsprechen exakt dem KI-Review. Ein damit feingetuntes Modell kann direkt als Chat-Modell eingetragen werden.

## Eigener Detektor-Dienst

`POST multipart/form-data` mit `image` (PNG) und `labels` → Antwort
`{"detections":[{"label":"hand","box":[x1,y1,x2,y2],"score":0.9}]}` (Pixelkoordinaten).

## Entwicklung

```bash
# Backend (braucht Postgres + Redis)
cd backend && go run -mod=vendor ./cmd/server
go test -mod=vendor ./...
# Frontend (Proxy auf :8080)
cd frontend && npm install && npm run dev
```

Gestaltung: Designtokens (Abstände im 8-px-Raster, Radien, Elevation, Flächen-/Linienfarben, Typo) stehen am Anfang von `frontend/src/app.css`. Schriften (IBM Plex Sans/Condensed/Mono) sind selbst gehostet (`@fontsource`), damit die Oberfläche auch ohne Internet vollständig aussieht.
