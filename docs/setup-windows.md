# Setup on a low-end Windows laptop (no Docker Desktop)

**Idea:** run Linux inside WSL2 and install the plain **Docker Engine** there (what servers run). No Docker
Desktop VM, no GUI, no licence questions, ~150 MB RAM for our dev containers. The Go app itself runs natively
in WSL (faster than in a container, instant hot reload). The same `compose` files work unchanged on servers.

Alternatives if you prefer a GUI: Podman Desktop or Rancher Desktop (both free). They still run a VM and use
more memory than the approach below.

## 1. Install WSL2 + Ubuntu (PowerShell as Administrator)
```powershell
wsl --install -d Ubuntu-24.04
```
Reboot, open "Ubuntu" from the Start menu, create your Linux user.

## 2. Cap WSL memory (important on 8 GB laptops)
Create `C:\Users\<you>\.wslconfig`:
```ini
[wsl2]
memory=3GB          # raise to 4GB if you have 16 GB
processors=2
swap=2GB
[experimental]
autoMemoryReclaim=gradual   # gives RAM back to Windows when idle
sparseVhd=true              # disk image shrinks automatically
```
Then in PowerShell: `wsl --shutdown` (it restarts with the new limits next time).

## 3. Inside Ubuntu: tools
```bash
sudo apt update && sudo apt -y upgrade
sudo apt -y install build-essential git curl make ffmpeg
# Docker Engine (official script) – no Docker Desktop
curl -fsSL https://get.docker.com | sudo sh
sudo usermod -aG docker $USER && newgrp docker
docker run --rm hello-world
```
Systemd is on by default in current WSL Ubuntu, so Docker starts automatically. If not:
`echo -e "[boot]\nsystemd=true" | sudo tee /etc/wsl.conf` and `wsl --shutdown`.

Go (latest stable from go.dev/dl, e.g. 1.27.x):
```bash
GO=go1.27.4   # check https://go.dev/dl for the newest
curl -fsSL https://go.dev/dl/$GO.linux-amd64.tar.gz | sudo tar -C /usr/local -xz
echo 'export PATH=$PATH:/usr/local/go/bin:$HOME/go/bin' >> ~/.bashrc && source ~/.bashrc
go version
```
Node.js is only needed for browser tests (`make test-e2e`): `curl -fsSL https://deb.nodesource.com/setup_lts.x | sudo -E bash - && sudo apt -y install nodejs`.

## 4. Keep code INSIDE Linux
Clone into `~/code/...` (Linux filesystem), never `/mnt/c/...` – file watching and builds are 5-10× faster.
```bash
mkdir -p ~/code && cd ~/code
git clone <your-new-repo-url> myapp && cd myapp
make setup     # tailwind binary, go tools, .env with fresh secrets, first build
make dev       # db + mailpit in Docker, app with hot reload
```
Open http://localhost:8080 in your Windows browser (WSL forwards localhost). Emails: http://localhost:8025.

## 5. VS Code + Claude Code
1. Install VS Code on Windows and the **WSL** extension (Microsoft).
2. In the Ubuntu terminal, inside your project: `code .` → VS Code opens connected to WSL
   (bottom-left shows "WSL: Ubuntu").
3. Extensions view → install **Claude Code** (and **Go**, **templ-vscode**, **Tailwind CSS IntelliSense**) – VS Code
   installs them on the WSL side. Sign in to Claude when prompted.
4. Open the Claude Code panel (Spark icon / Command Palette → "Claude Code: Open"). It reads `CLAUDE.md` automatically.
5. Recommended VS Code settings (`.vscode/settings.json` is included).

## Daily use
| Task | Command |
|---|---|
| start working | `make dev` (Ctrl+C to stop the app; containers keep running) |
| stop containers | `docker compose stop` |
| run all checks | `make check` |
| full production-like stack locally | `make up` / `make down` |
| free RAM when done | `wsl --shutdown` (PowerShell) |

## Troubleshooting
- `permission denied … docker.sock` → you skipped `usermod -aG docker`; log out/in of WSL.
- Port 8080 busy → change `ADDR` in `.env`.
- Very slow builds → project is under `/mnt/c`; move it to `~/code`.
- WSL using too much RAM → check `.wslconfig`, then `wsl --shutdown`.
