# Getting started on Windows (low-end laptop) with VS Code + Claude Code

Plan: **only the database and mail catcher run in Docker**. Go, Flutter, VS Code and Chrome run natively on
Windows. Docker Engine runs inside WSL2 *without* Docker Desktop. The same compose files and images run on
the Linux server. Idle cost on your laptop: roughly 300–500 MB RAM for WSL + Postgres + Mailpit.

---
## Part A — One-time machine setup (≈45 min)

### A1. Windows tools (PowerShell)
```powershell
winget install Git.Git
winget install GoLang.Go
winget install Task.Task
winget install Microsoft.VisualStudioCode
winget install Google.Chrome
```
**Flutter**: follow https://docs.flutter.dev/get-started/install/windows (unzip the SDK to `C:\dev\flutter`,
add `C:\dev\flutter\bin` to PATH) — or in VS Code install the Flutter extension and use
*"Flutter: New Project" → Download SDK*. Then run `flutter doctor`.
Android: on a weak laptop **skip the emulator**. Use your phone: enable Developer options → USB debugging,
plug in, `flutter devices`. You still need the Android SDK command-line tools (Android Studio installs them;
you can close Studio afterwards and never open it again).

### A2. Lightweight Docker: WSL2 + Docker Engine (no Docker Desktop)
1. PowerShell **as Administrator**: `wsl --install -d Ubuntu-24.04`, reboot, create your Linux user.
2. Cap WSL's memory. Create `C:\Users\<you>\.wslconfig`:
   ```ini
   [wsl2]
   # use 2GB on an 8 GB laptop
   memory=3GB
   processors=2
   swap=2GB
   # Windows and WSL share localhost (Windows 11 22H2+)
   networkingMode=mirrored

   [experimental]
   # give RAM back to Windows when idle / shrink the Linux disk after deletes
   autoMemoryReclaim=gradual
   sparseVhd=true
   ```
   Then `wsl --shutdown` and reopen Ubuntu. (On Windows 10 remove `networkingMode`; WSL still forwards
   container ports to `localhost`.)
3. In the **Ubuntu** terminal:
   ```bash
   curl -fsSL https://get.docker.com | sudo sh
   sudo usermod -aG docker $USER
   exit
   ```
   Reopen Ubuntu, check `docker run --rm hello-world`. (systemd is on by default in current WSL; if Docker
   doesn't start, add `[boot]\nsystemd=true` to `/etc/wsl.conf` and `wsl --shutdown`.)
4. From Windows: `wsl docker ps` must work — the Taskfile uses that automatically on Windows.
5. Free all that RAM any time: `wsl --shutdown`.

Alternatives if you prefer a GUI: Rancher Desktop or Podman Desktop (both lighter than Docker Desktop).
This kit assumes plain Docker Engine in WSL because it's the lightest and identical to the server.

### A3. VS Code + Claude Code
1. Open VS Code → Extensions (`Ctrl+Shift+X`) → install **Claude Code** (Anthropic), **Go**, **Flutter**.
   (The repo also recommends them via `.vscode/extensions.json`.)
2. Click the Spark icon (Activity Bar or top-right of an open file) → sign in with your Claude account.
3. Useful keys: `Ctrl+Esc` toggles focus editor↔Claude · `Alt+K` inserts an @-reference to your selection ·
   `Shift+Tab` in the prompt cycles permission modes incl. **Plan mode** · type `/` for commands.

---
## Part B — Create your project from the kit (5 min)
```powershell
mkdir C:\dev; cd C:\dev
# unzip starter-kit.zip here, then rename the folder to your project name:
ren starter-kit myapp; cd myapp
git init; git add .; git commit -m "chore: start from starter kit"
# create an empty private repo on GitHub, then:
git remote add origin https://github.com/<you>/myapp.git
git branch -M main; git push -u origin main
```
Keep the project on the Windows drive (`C:\dev\…`): Go/Flutter run natively and only Postgres data lives in WSL.

---
## Part C — First run (10 min)
```powershell
task setup        # .env, go mod tidy (creates go.sum), flutter create (android/ios/web scaffolding), pub get
task infra:up     # Postgres + Mailpit in WSL Docker
task api          # terminal 1 → http://localhost:8080/healthz
task web          # terminal 2 → Chrome opens http://localhost:5000
```
Sign in with any email → open **http://localhost:8025** (Mailpit) → copy the code → you're in.
Try Notes (search, filter, offline: DevTools → Network → Offline, then edit). For file uploads also run
`task infra:up:media` (local S3). Run the tests: `task test` and `task test:api`.
Commit `backend/go.sum`, `frontend/pubspec.lock` and the generated `frontend/android`, `ios`, `web/icons`.

---
## Part D — Build your app with Claude Code (the cheap-on-tokens way)

### D1. Start
Open the folder in VS Code → Claude panel → paste (edit the idea):
```text
/new-app <describe your app idea in 3–8 sentences: who uses it, what they do, which countries,
web/mobile/both, anything sensitive like health, payments, kids, location>
```
Example:
```text
/new-app A booking app for independent yoga teachers. Teachers publish classes (time, place, capacity, price);
students search by city and date, book a spot, get email reminders and can cancel up to 12h before.
Teachers check attendees at the studio even with bad signal. Payments later, not in v1. Launch in Germany
and Austria. Students mostly use the website, teachers the mobile app.
```
Claude interviews you briefly, writes `docs/APP.md` (scope, entities, PII/compliance delta, milestones) and
**stops for your approval**. Edit the file if needed, then say: `approved — do milestone 1`.

If skills ever fail to load, paste this instead:
```text
Read CLAUDE.md and .claude/skills/new-app/SKILL.md, then follow that workflow for this app idea: <idea>
```

### D2. Loop per milestone
1. `approved — do milestone N` (Claude implements, runs the milestone's check, fixes).
2. Review the diff in VS Code; run the app yourself.
3. `git add -A && git commit -m "feat: milestone N"` (or ask Claude to commit).
4. **`/clear`** before the next milestone. `docs/APP.md` → "Progress" carries the state, so a fresh session
   starts cheap and focused.
Other useful skills: `/add-entity Booking …`, `/security-review`, `/ship-check`.

### D3. Token-saving rules (they also make Claude more accurate)
- **One task per session; `/clear` between tasks.** Long mixed sessions cost more and drift.
- **Point, don't paste**: `@backend/internal/notes/notes.go` or "follow the notes module" beats explaining.
- **Plan mode only for multi-file or unclear work**; for one-line fixes ask directly.
- **Give a check**: "run `go test ./internal/bookings/...` and fix until green" — avoids back-and-forth.
- **Small precise prompts with the symptom + location + what 'fixed' means.**
- `/compact keep changed files and test commands` when a session must continue; `/context` shows what's using space.
- After 2 failed corrections: `/clear` and re-prompt with what you learned.
- Pick a smaller/faster model for routine edits and the strongest one for planning/security (`/model`).
- Let Claude use subagents for broad investigation ("use a subagent to find where X happens") — keeps the main context small.
- The repo is built for this: short `CLAUDE.md` index, docs split by topic, one reference module to copy,
  `.claude/settings.json` blocks reading build output/lockfiles/secrets, Taskfile gives one-word commands.

---
## Part E — Go live
Follow `docs/DEPLOYMENT.md` (VPS setup script, Cloudflare DNS, R2, email domain, GitHub secrets, `git tag v1.0.0`).
Before launch: `/ship-check` and the checklist in `docs/compliance/COMPLIANCE.md`.

## Troubleshooting
| Symptom | Fix |
|---|---|
| `wsl docker` not found | open Ubuntu once; `docker ps` inside it; re-run A2.3 |
| API can't reach Postgres | `task infra:up`; `wsl docker ps`; port 5432 used by a Windows Postgres? stop it |
| Web app CORS error | run web on port 5000 (`task web`) or add your port to `CORS_ORIGINS` |
| Android can't reach API | emulator: `task mobile` (10.0.2.2); phone: `--dart-define=API_BASE_URL=http://<PC-LAN-IP>:8080` and allow port 8080 in Windows Firewall |
| Laptop slow | `wsl --shutdown` when not coding; lower `memory=` in `.wslconfig`; close Android Studio |
| `go.sum` missing in Docker build | run `task setup` (go mod tidy) and commit `go.sum` |
