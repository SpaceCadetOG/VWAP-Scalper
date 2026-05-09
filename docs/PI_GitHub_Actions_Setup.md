# Raspberry Pi GitHub Actions Setup

## Goal
Deploy `cmd/live` to Raspberry Pi automatically from `main`.

## 1) GitHub Secrets (Repository -> Settings -> Secrets and variables -> Actions)

- `PI_HOST`: Pi host/IP
- `PI_USER`: SSH user (for example `pi`)
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

## 3) Required .env values for your request

```bash
BOT_SYMBOLS=BTCUSDT
BOT_NOTIONAL_USD=10
BOT_TARGET_LEVERAGE=10
PAPER_AUTO_PROMOTE_LIVE=true
PAPER_PROMOTE_LIVE_NOTIONAL_USD=10
SIM_USE_LIVE_SNAPSHOT=true
```

## 4) Deploy

- Push to `main` or run manual `deploy-pi` workflow.
- Workflow builds linux arm64 binary and restarts `vwap-scalper.service`.

## 5) Verify on Pi

```bash
sudo systemctl status vwap-scalper.service --no-pager -l
tail -f /opt/vwap-scalper/out/live.log
```

