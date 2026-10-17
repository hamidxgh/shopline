package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
)

func readVaultDBPassword() (string, error) {
	addr := os.Getenv("VAULT_ADDR")
	if addr == "" {
		addr = "http://localhost:8200"
	}
	token := os.Getenv("VAULT_TOKEN")
	if token == "" {
		token = "root"
	}

	req, err := http.NewRequest(http.MethodGet, addr+"/v1/secret/data/shopline", nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("X-Vault-Token", token)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("vault status %d", resp.StatusCode)
	}

	var body struct {
		Data struct {
			Data map[string]string `json:"data"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return "", err
	}
	pw := body.Data.Data["db_password"]
	if pw == "" {
		return "", fmt.Errorf("db_password missing in vault")
	}
	return pw, nil
}
