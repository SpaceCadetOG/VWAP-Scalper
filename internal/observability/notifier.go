package observability

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"
)

type Notifier struct {
	discordWebhook string
	telegramBotTok string
	telegramChatID string
	client         *http.Client
}

func NewNotifierFromEnv() *Notifier {
	return &Notifier{
		discordWebhook: strings.TrimSpace(os.Getenv("ALERT_DISCORD_WEBHOOK_URL")),
		telegramBotTok: strings.TrimSpace(os.Getenv("ALERT_TELEGRAM_BOT_TOKEN")),
		telegramChatID: strings.TrimSpace(os.Getenv("ALERT_TELEGRAM_CHAT_ID")),
		client:         &http.Client{Timeout: 6 * time.Second},
	}
}

func (n *Notifier) Enabled() bool {
	return n.discordWebhook != "" || (n.telegramBotTok != "" && n.telegramChatID != "")
}

func (n *Notifier) Send(event, text string) error {
	msg := fmt.Sprintf("[%s] %s", strings.ToUpper(strings.TrimSpace(event)), strings.TrimSpace(text))
	var errs []string
	if n.discordWebhook != "" {
		if err := n.sendDiscord(msg); err != nil {
			errs = append(errs, "discord:"+err.Error())
		}
	}
	if n.telegramBotTok != "" && n.telegramChatID != "" {
		if err := n.sendTelegram(msg); err != nil {
			errs = append(errs, "telegram:"+err.Error())
		}
	}
	if len(errs) > 0 {
		return errors.New(strings.Join(errs, "; "))
	}
	return nil
}

func (n *Notifier) sendDiscord(msg string) error {
	b, _ := json.Marshal(map[string]string{"content": msg})
	req, _ := http.NewRequest(http.MethodPost, n.discordWebhook, bytes.NewReader(b))
	req.Header.Set("content-type", "application/json")
	resp, err := n.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode/100 != 2 {
		out, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("status=%d body=%s", resp.StatusCode, string(out))
	}
	return nil
}

func (n *Notifier) sendTelegram(msg string) error {
	u := fmt.Sprintf("https://api.telegram.org/bot%s/sendMessage", n.telegramBotTok)
	form := url.Values{}
	form.Set("chat_id", n.telegramChatID)
	form.Set("text", msg)
	req, _ := http.NewRequest(http.MethodPost, u, strings.NewReader(form.Encode()))
	req.Header.Set("content-type", "application/x-www-form-urlencoded")
	resp, err := n.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode/100 != 2 {
		out, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("status=%d body=%s", resp.StatusCode, string(out))
	}
	return nil
}
