package clientengine

import (
	"fmt"
	"net/url"
	"strconv"
	"strings"
)

type ParsedConfigURI struct {
	Name     string
	Peer     string
	Hashes   string
	Password string
	Port     string
	Workers  int
}

// ParseConfigURI разбирает конфигурационную ссылку qwdtt:// или wdtt://
func ParseConfigURI(raw string) (*ParsedConfigURI, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, fmt.Errorf("пустая ссылка")
	}

	// 1. Формат qwdtt://config?... или qwdtt://?...
	if strings.HasPrefix(raw, "qwdtt://") {
		uStr := raw
		// Приводим к стандартному URL виду для net/url
		uStr = "http://" + strings.TrimPrefix(uStr, "qwdtt://")
		u, err := url.Parse(uStr)
		if err != nil {
			return nil, fmt.Errorf("ошибка парсинга qwdtt ссылки: %w", err)
		}

		q := u.Query()
		res := &ParsedConfigURI{
			Name:     q.Get("name"),
			Peer:     q.Get("peer"),
			Hashes:   q.Get("hashes"),
			Password: q.Get("pass"),
			Port:     q.Get("port"),
		}
		if res.Hashes == "" {
			res.Hashes = q.Get("hash")
		}
		if res.Password == "" {
			res.Password = q.Get("password")
		}
		if wStr := q.Get("workers"); wStr != "" {
			if w, err := strconv.Atoi(wStr); err == nil && w > 0 {
				res.Workers = w
			}
		}
		return res, nil
	}

	// 2. Формат wdtt://host:port:wgPort:localPort:password:vkHash
	if strings.HasPrefix(raw, "wdtt://") {
		trimmed := strings.TrimPrefix(raw, "wdtt://")
		parts := strings.Split(trimmed, ":")
		if len(parts) >= 6 {
			// host:port:wgPort:localPort:password:vkHash
			host := parts[0]
			dtlsPort := parts[1]
			localPort := parts[3]
			password := parts[4]
			vkHash := parts[5]

			return &ParsedConfigURI{
				Name:     "WDTT-Server",
				Peer:     fmt.Sprintf("%s:%s", host, dtlsPort),
				Hashes:   vkHash,
				Password: password,
				Port:     localPort,
			}, nil
		}
		return nil, fmt.Errorf("неверный формат wdtt:// ссылки (ожидалось 6 параметров)")
	}

	return nil, fmt.Errorf("неизвестная схема ссылки (поддерживаются qwdtt:// и wdtt://)")
}
