package clientengine

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"
)

// VKHashCheckResult содержит структурированный результат проверки хеша звонка VK
type VKHashCheckResult struct {
	Hash         string   `json:"hash"`
	Normalized   string   `json:"normalized"`
	Status       string   `json:"status"` // "ok", "expired", "captcha", "error"
	TurnURLs     []string `json:"turn_urls"`
	TurnCount    int      `json:"turn_count"`
	LatencyMs    int64    `json:"latency_ms"`
	ErrorMessage string   `json:"error_message"`
	Hint         string   `json:"hint"`
}

// CheckVKCallHash проверяет жизнеспособность одного хеша или ссылки VK-звонка
func CheckVKCallHash(ctx context.Context, rawHash string) *VKHashCheckResult {
	res := &VKHashCheckResult{
		Hash:   rawHash,
		Status: "error",
	}

	normalized := normalizeVKJoinHash(rawHash)
	res.Normalized = normalized

	if normalized == "" {
		res.ErrorMessage = "Пустой или некорректный хеш"
		res.Hint = "Вставьте валидную ссылку вида https://vk.com/call/join/... или хеш звонка"
		return res
	}

	start := time.Now()
	// Проверяем через VKCalls API путь без вмешательства в рабочий кэш
	_, _, turnURLs, err := getVKCredsViaVKCallsPath(ctx, normalized, 9990)
	res.LatencyMs = time.Since(start).Milliseconds()

	if err == nil && len(turnURLs) > 0 {
		res.Status = "ok"
		res.TurnURLs = turnURLs
		res.TurnCount = len(turnURLs)
		res.ErrorMessage = ""
		res.Hint = fmt.Sprintf("Хеш активен! Получено %d релеев TURN (задержка %d мс)", len(turnURLs), res.LatencyMs)
		return res
	}

	if err != nil {
		errLower := strings.ToLower(err.Error())

		if callErr, ok := asCallUnavailableError(err); ok {
			res.Status = "expired"
			res.ErrorMessage = fmt.Sprintf("Звонок недоступен (код %d: %s)", callErr.Code, callErr.Message)
			res.Hint = "Комната звонка завершена или удалена. Создайте новый звонок в VK Звонки (vk.com/calls) и обновите ссылку."
			return res
		}

		var captchaErr *VkCaptchaError
		if errors.As(err, &captchaErr) {
			res.Status = "captcha"
			res.ErrorMessage = "VK запросил капчу для вашего IP"
			res.Hint = "Слишком много запросов к VK. Подождите 5-10 минут или смените IP."
			return res
		}

		var failure *vkCallsFailure
		if errors.As(err, &failure) {
			switch failure.Kind {
			case vkCallsFailureCall:
				res.Status = "expired"
				res.ErrorMessage = "Звонок завершён или ссылка не найдена"
				res.Hint = "Создайте новую ссылку в VK Звонки (vk.com/calls) и вставьте в слот."
				return res
			case vkCallsFailureNetwork:
				res.Status = "error"
				res.ErrorMessage = fmt.Sprintf("Сетевая ошибка при обращении к VK: %v", failure.Err)
				res.Hint = "Проверьте физическое подключение к интернету или доступность api.vk.me."
				return res
			default:
				res.Status = "error"
				res.ErrorMessage = failure.Error()
				res.Hint = "Ошибка VK API. Проверьте правильность ссылки звонка."
				return res
			}
		}

		if strings.Contains(errLower, "call_unavailable") || strings.Contains(errLower, "not found") || strings.Contains(errLower, "951") || strings.Contains(errLower, "954") {
			res.Status = "expired"
			res.ErrorMessage = "Звонок завершён или комната закрыта"
			res.Hint = "Ссылка звонка протухла. Сгенерируйте новую ссылку в приложении или на сайте VK Звонки."
			return res
		}

		res.Status = "error"
		res.ErrorMessage = err.Error()
		res.Hint = "Не удалось получить TURN-креды. Проверьте ссылку звонка."
		return res
	}

	res.Status = "error"
	res.ErrorMessage = "VK не вернул TURN-релеи"
	res.Hint = "Попробуйте создать новую ссылку звонка в VK"
	return res
}

// CheckMultipleVKCallHashes параллельно проверяет список хешей
func CheckMultipleVKCallHashes(ctx context.Context, hashes []string) []*VKHashCheckResult {
	results := make([]*VKHashCheckResult, len(hashes))
	if len(hashes) == 0 {
		return results
	}

	doneCh := make(chan struct {
		idx int
		res *VKHashCheckResult
	}, len(hashes))

	for i, h := range hashes {
		go func(idx int, raw string) {
			checkCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
			defer cancel()
			r := CheckVKCallHash(checkCtx, raw)
			doneCh <- struct {
				idx int
				res *VKHashCheckResult
			}{idx: idx, res: r}
		}(i, h)
	}

	for i := 0; i < len(hashes); i++ {
		item := <-doneCh
		results[item.idx] = item.res
	}

	return results
}
