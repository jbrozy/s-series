# S-Check – semantische Prüfung für S-Series-Dokumentation

## Starten

```bash
cp .env.example .env      # Passwörter anpassen (optional)
docker compose up -d --build
```

Danach `http://localhost:8080` öffnen. Der erste registrierte Benutzer wird Administrator.
Zum Ausprobieren `testdata/beispiel-export.zip` in einem neuen Projekt hochladen (enthält absichtlich Fehler).

Lokale Modelle ohne Internet: `docker compose --profile ollama up -d`, dann unter
*Administration → Modelle* `http://ollama:11434/v1` eintragen.

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
| `backend/` | Go-API + Worker. Abhängigkeiten liegen in `vendor/` – Build funktioniert offline. |
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

Dismissed-Befunde bleiben in späteren Analysen des Projekts verworfen. Befunde lassen sich als CSV exportieren.

## Trainingsdaten

*Projekt → Trainingsdaten* erzeugt JSONL (Chat- oder Alpaca-Format) aus korrekten Datenmodulen, in die typische Fehler eingebaut werden. Prompt und Antwortformat entsprechen exakt dem KI-Review – ein damit feingetuntes Modell kann direkt als Chat-Modell eingetragen werden.

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
