package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

type authBridge struct {
	URL    string
	Secret string
	Client *http.Client
}

func (b authBridge) authorized(r *http.Request) bool {
	return r.Header.Get("Authorization") == "Bearer "+b.Secret
}

func (b authBridge) call(path string, payload any) (map[string]any, int, error) {
	raw, err := json.Marshal(payload)
	if err != nil {
		return nil, 0, err
	}
	req, err := http.NewRequest(http.MethodPost, strings.TrimRight(b.URL, "/")+path, bytes.NewReader(raw))
	if err != nil {
		return nil, 0, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+b.Secret)
	client := b.Client
	if client == nil {
		client = &http.Client{Timeout: 8 * time.Second}
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, 0, err
	}
	defer resp.Body.Close()
	var body map[string]any
	if err := json.NewDecoder(io.LimitReader(resp.Body, 64<<10)).Decode(&body); err != nil {
		return nil, resp.StatusCode, fmt.Errorf("decode auth bridge response: %w", err)
	}
	return body, resp.StatusCode, nil
}

func (a *app) apiAuthRequest(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Phone string `json:"phone"`
	}
	if json.NewDecoder(http.MaxBytesReader(w, r.Body, 8<<10)).Decode(&body) != nil {
		writeJSONStatus(w, 400, map[string]any{"error": "JSON_INVALID"})
		return
	}
	result, status, err := a.auth.call("/v1/deltament/auth/request", body)
	if err != nil {
		writeJSONStatus(w, 503, map[string]any{"error": "Не удалось связаться с сервисом авторизации."})
		return
	}
	if status >= 400 {
		result["error"] = authErrorMessage(fmt.Sprint(result["error"]))
	}
	writeJSONStatus(w, status, result)
}

func (a *app) apiAuthVerify(w http.ResponseWriter, r *http.Request) {
	var body struct {
		RequestID string `json:"request_id"`
		Code      string `json:"code"`
	}
	if json.NewDecoder(http.MaxBytesReader(w, r.Body, 8<<10)).Decode(&body) != nil {
		writeJSONStatus(w, 400, map[string]any{"error": "JSON_INVALID"})
		return
	}
	result, status, err := a.auth.call("/v1/deltament/auth/verify", body)
	if err != nil {
		writeJSONStatus(w, 503, map[string]any{"error": "Сервис авторизации временно недоступен."})
		return
	}
	if status >= 200 && status < 300 {
		uid, ok := result["user_id"].(float64)
		if !ok || uid <= 0 || uid != float64(int64(uid)) {
			writeJSONStatus(w, 502, map[string]any{"error": "AUTH_INVALID_RESPONSE"})
			return
		}
		var id int64
		var phone, username, first, last string
		if err := a.db.QueryRow(r.Context(), `SELECT id, phone, username, first_name, last_name FROM users WHERE id=$1 AND deleted_at IS NULL`, int64(uid)).Scan(&id, &phone, &username, &first, &last); err != nil {
			writeJSONStatus(w, 401, map[string]any{"error": "Аккаунт DeltaGram не найден или недоступен."})
			return
		}
		a.setSession(w, id)
		result = map[string]any{"ok": true, "user": map[string]any{"id": id, "phone": phone, "username": username, "display": strings.TrimSpace(first + " " + last), "isAdmin": false}}
	} else {
		result["error"] = authErrorMessage(fmt.Sprint(result["error"]))
	}
	writeJSONStatus(w, status, result)
}

func writeJSONStatus(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func authErrorMessage(code string) string {
	switch code {
	case "PHONE_INVALID":
		return "Введите корректный внутренний номер DeltaGram."
	case "ACCOUNT_NOT_LINKED":
		return "Этот номер не привязан к @deltagram_robot. Откройте бота и привяжите аккаунт."
	case "AUTH_UPSTREAM_UNAVAILABLE":
		return "Не удалось проверить номер на сервере DeltaGram. Попробуйте ещё раз позже."
	case "AUTH_RATE_LIMITED":
		return "Подождите минуту перед повторным запросом кода."
	case "AUTH_CODE_INVALID":
		return "Неверный код. Проверьте сообщение от @deltagram_robot."
	case "AUTH_CHALLENGE_EXPIRED":
		return "Срок действия кода истёк. Запросите новый код."
	case "AUTH_TOO_MANY_ATTEMPTS":
		return "Слишком много неудачных попыток. Запросите новый код."
	case "AUTH_CHALLENGE_NOT_FOUND":
		return "Запрос кода не найден. Начните вход заново."
	case "CODE_INVALID":
		return "Введите 5-значный код из сообщения бота."
	default:
		return "Не удалось выполнить вход. Попробуйте ещё раз позже."
	}
}
