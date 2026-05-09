# Raspberry Pi Deployment Setup

## Goal
Deploy `cmd/live` to Raspberry Pi, either manually from your Go machine or automatically from GitHub Actions.

## 1) GitHub Secrets (Repository -> Settings -> Secrets and variables -> Actions)

- `PI_HOST`: Pi host/IP
- `PI_USER`: SSH user (for example `traderbot`)
- `PI_SSH_KEY`: private key content for SSH
- `PI_APP_DIR`: app directory on Pi (for example `/opt/vwap-scalper`)

## 2) Prepare Pi once

```bash
sudo mkdir -p /opt/vwap-scalper/out
sudo chown -R $USER:$USER /opt/vwap-scalper
cp /path/to/your/.env /opt/vwap-scalper/.env
```

Copy service file:

```bash
sudo cp deploy/vwap-scalper.service /etc/systemd/system/vwap-scalper.service
sudo systemctl daemon-reload
sudo systemctl enable vwap-scalper.service
```

The service file in this repo assumes the runtime user is `traderbot`.

## 3) Required .env values for your request

```bash
BOT_SYMBOL_SOURCE_MODE=dynamic
BOT_SYMBOLS_MAX=25
BOT_SYMBOL_REFRESH_SEC=600
BOT_NOTIONAL_USD=10
BOT_TARGET_LEVERAGE=3
PAPER_AUTO_PROMOTE_LIVE=true
PAPER_PROMOTE_LIVE_NOTIONAL_USD=10
SIM_USE_LIVE_SNAPSHOT=true
```

Notes:
- Set `BOT_SYMBOL_SOURCE_MODE=static` and `BOT_SYMBOLS=BTCUSDT,ETHUSDT` if you want a fixed list instead of venue discovery.
- Dynamic mode now discovers and merges eligible assets across Hyperliquid, Aster, and Lighter automatically.
- `BOT_SYMBOLS_MAX=0` means no cap, but use that carefully because scanning too many assets can increase API pressure and trigger rate limits.

## 4) Manual Deploy From Go Machine

Build on your Mac or Go machine:

```bash
cd /path/to/VWAP-Scalper
GOOS=linux GOARCH=arm64 CGO_ENABLED=0 go build -o dist/vwap-live ./cmd/live
```

Copy the binary to the Pi and install it:

```bash
scp dist/vwap-live traderbot@192.168.3.28:/tmp/vwap-live
ssh traderbot@192.168.3.28 'sudo install -m 0755 /tmp/vwap-live /opt/vwap-scalper/vwap-live'
```

Restart and verify:

```bash
ssh traderbot@192.168.3.28 'sudo systemctl restart vwap-scalper.service && sudo systemctl status vwap-scalper.service --no-pager -l | head -n 40'
```

## 5) GitHub Actions Deploy

- Push to `main` or run manual `deploy-pi` workflow.
- Workflow builds the linux arm64 binary and restarts `vwap-scalper.service`.

## 6) Verify on Pi

```bash
sudo systemctl status vwap-scalper.service --no-pager -l
tail -f /opt/vwap-scalper/out/live.log
```
