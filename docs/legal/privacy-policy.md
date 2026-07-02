# Privacy Policy

**Effective date:** February 12, 2026
**Last updated:** February 12, 2026

CH-UI ("we", "our", "us") is developed by Caio Ricciuti. This privacy policy explains how we handle data when you use CH-UI software.

---

## What CH-UI does NOT collect

CH-UI is a self-hosted application. When you run CH-UI on your own infrastructure:

- **No telemetry** is sent to us or any third party
- **No usage data** leaves your server
- **No analytics** are collected
- **No cookies** are set by us (only session cookies for your own login)
- **Your queries, data, and database contents never leave your infrastructure**

## Data stored locally

CH-UI stores the following data in a local SQLite database on your server:

- **User sessions** — login tokens for authenticated access
- **Saved queries** — queries you choose to save
- **Dashboard configurations** — layout and panel settings (Pro)
- **Scheduled jobs** — query schedules you create (Pro)
- **Connection settings** — ClickHouse connection details (encrypted)
- **License information** — your license key if you activate Pro
- **Application settings** — preferences and configuration

All data is stored in the SQLite file specified by `database_path` in your config (default: `./data/ch-ui.db`). You have full control over this data.

## Pro license activation

When you activate a Pro license, the license file is stored locally in your database. No information is sent to external servers during activation — the license is validated offline using cryptographic signatures.

## Managed hosting

If you use a managed CH-UI hosting offering:

- We may store your account information (email, name) for authentication
- We may store your ClickHouse connection metadata (not your database contents)
- We do not access, read, or store your ClickHouse data
- Tunnel connections are end-to-end between your agent and your browser session

## Third-party services

The self-hosted CH-UI binary does not communicate with any third-party services except the ones you explicitly configure or invoke:

- **Your ClickHouse server(s)** — as configured by you
- **Your chosen LLM provider** — only if you enable the Brain AI feature (Pro) with your own API key. This may be OpenAI, any OpenAI-compatible API (e.g. Groq, Together), or a self-hosted/local model via Ollama. Brain sends schema metadata and, when you run a generated query, query context to that provider. No LLM provider is contacted unless Brain is configured.
- **GitHub** — only when you run `ch-ui update` (checks for and downloads new releases) or enable the optional GitHub model-sync integration.
- **Your chosen email/alert provider** — only if you configure alert channels (SMTP, Resend, or Brevo) to deliver governance and monitoring notifications.

CH-UI sends **no telemetry, analytics, or usage data** to us or any third party. Every outbound connection above is to a destination you configure, and can be avoided by not enabling the corresponding feature.

## Data deletion

Since all data is stored locally:

- Delete the SQLite database file to remove all application data
- Uninstall the binary to fully remove CH-UI

## Contact

For privacy questions: **me@caioricciuti.com**

## Changes

We may update this policy. Changes will be posted in this file and noted in release changelogs.
