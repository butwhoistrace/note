# Projektanalyse: note

## Überblick

**note** ist eine minimalistische, terminalbasierte Notiz- und Wissensdatenbank-Anwendung, geschrieben in **Go (1.22.2)**. Das Projekt setzt vollständig auf die Go-Standardbibliothek — es gibt **keine externen Abhängigkeiten**. Alles wird lokal gespeichert, ohne Cloud-Anbindung.

- **Autor:** butwhoistrace
- **Lizenz:** MIT (2026)
- **Umfang:** ~2.150 Zeilen Code
- **Binary-Größe:** ~2,4 MB

---

## Projektstruktur

```
note/
├── main.go                    # CLI-Einstiegspunkt & Kommando-Dispatcher (~1.136 Zeilen)
├── internal/
│   ├── store/store.go         # Notiz-Speicherung, CRUD-Operationen (~422 Zeilen)
│   ├── index/index.go         # Volltextsuche & Indexierung (~379 Zeilen)
│   ├── crypto/crypto.go       # AES-256-GCM Verschlüsselung (~99 Zeilen)
│   └── display/display.go     # Terminal-Formatierung mit ANSI-Farben (~117 Zeilen)
├── go.mod                     # Go-Modul-Konfiguration
├── README.md                  # Dokumentation
├── LICENSE                    # MIT-Lizenz
└── gifani.gif                 # Demo-Animation
```

---

## Kernfunktionen

### Notiz-Verwaltung
- **Erstellen** (`new`, `quick`) — Neue Notizen mit optionalen Tags anlegen
- **Bearbeiten** (`add`, `edit`, `rm`) — Inhalte anhängen, Zeilen ersetzen oder entfernen
- **Anzeigen** (`show`) — Notizen mit Zeilennummern im Terminal darstellen
- **Löschen/Wiederherstellen** (`delete`, `restore`) — Papierkorb-System
- **Umbenennen** (`rename`)

### Suche
- **Volltextsuche** mit Tokenisierung (invertierter Index, JSON-basiert)
- **Fuzzy-Suche** (Levenshtein-Distanz ≤ 2)
- **Regex-Suche** für Muster-basierte Abfragen
- **Kontextanzeige** (umliegende Zeilen)
- **Tag-basierte Filterung**

### Organisation
- Sortierte Notiz-Listen (nach Datum oder Name)
- Tag-Management (hinzufügen, entfernen, auflisten)
- **Baumansicht** gruppiert nach Tags
- **Timeline-Ansicht** (chronologisch)

### Sicherheit & Verschlüsselung
- **AES-256-GCM** Verschlüsselung einzelner Notizen
- **PBKDF2** Schlüsselableitung (100.000 Iterationen, SHA-256)
- Bulk-Verschlüsselung aller Notizen (`lock`/`unlock`)
- Speicherformat: `[16-Byte Salt][12-Byte Nonce][Ciphertext]`

### Synchronisation
- Git-Integration: `init`, `push` (sync), `pull`
- Ermöglicht verteilte Sicherung über Git-Repositories

### Export & Import
- Einzelne Notizen oder alle Notizen exportieren
- ZIP-Format für Massenexport
- Markdown-Import

### Automatisierung
- **Hook-System** für Events: `new`, `add`, `delete`, `sync`, `search`
- Benutzerdefinierte Shell-Befehle pro Event

### Wartung
- `reindex` — Suchindex neu aufbauen
- `doctor` — Gesundheitsprüfung
- `stats` — Statistiken (Anzahl, Tags, Speicher)
- Papierkorb-Verwaltung

---

## Speicherarchitektur

| Pfad | Zweck |
|------|-------|
| `~/.note/` | Basisverzeichnis |
| `~/.note/notes/` | Notizen als Markdown-Dateien |
| `~/.note/.trash/` | Papierkorb |
| `~/.note/.index` | JSON-Suchindex |
| `~/.note/.hooks` | Hook-Konfigurationen |

### Notiz-Format (Markdown mit Frontmatter)
```markdown
---
title: Notiz-Titel
tags: tag1, tag2, tag3
created: 2006-01-02
---
Inhalt beginnt hier...
```

---

## Performance

| Metrik | Wert |
|--------|------|
| Startzeit | ~15 ms |
| Suchzeit | ~20 ms |
| Speicherverbrauch | < 10 MB |
| Binary-Größe | ~2,4 MB |

---

## Bewertung

### Stärken
- **Null Abhängigkeiten** — Nur Go-Standardbibliothek, extrem wartungsarm
- **Kompakter Code** — ~2.150 Zeilen für ein vollständiges Feature-Set
- **Starke Verschlüsselung** — AES-256-GCM mit robusten Schlüsselableitungsparametern
- **Schnell** — Geringe Startzeit und schnelle Suchoperationen
- **Datenschutz** — Vollständig lokale Speicherung, keine Telemetrie
- **Erweiterbar** — Hook-System für individuelle Automatisierung
- **Saubere Architektur** — Klare Trennung in `store`, `index`, `crypto`, `display`

### Verbesserungspotenzial
- **Keine Tests** — Es gibt keine Unit- oder Integrationstests
- **Monolithische main.go** — Mit ~1.136 Zeilen könnte die Hauptdatei in kleinere Kommando-Module aufgeteilt werden
- **Keine CI/CD-Pipeline** — Kein automatisiertes Build/Test-System konfiguriert
- **Fehlende Konfigurationsdatei** — Einstellungen wie Basisverzeichnis sind hardcoded (`~/.note/`)

---

## Fazit

**note** ist eine durchdachte, minimalistische CLI-Anwendung, die zeigt, wie viel Funktionalität mit reinem Go und der Standardbibliothek erreichbar ist. Das Projekt bietet ein vollständiges Feature-Set für die lokale Notizverwaltung — von Volltextsuche über Verschlüsselung bis hin zu Git-Synchronisation — in einem kompakten, performanten Paket. Die größten nächsten Schritte wären das Hinzufügen von Tests und das Aufteilen der `main.go` in kleinere Module.
