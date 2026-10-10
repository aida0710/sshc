package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os/exec"
	"strings"
)

func mintBootstrap(ctx context.Context) (string, error) {
	openURL, err := exec.CommandContext(ctx, "sshc", "open").Output()
	if err != nil {
		return "", err
	}
	_, fragment, found := strings.Cut(strings.TrimSpace(string(openURL)), "#")
	if !found {
		return "", fmt.Errorf("bootstrap fragment missing")
	}
	return fragment, nil
}

func initialiseDemoVault(ctx context.Context, client *http.Client) error {
	fragment, err := mintBootstrap(ctx)
	if err != nil {
		return err
	}
	parameters, err := url.ParseQuery(fragment)
	if err != nil {
		return err
	}
	bootstrap, _ := http.NewRequestWithContext(ctx, http.MethodPost, engineOrigin+"/api/v1/session/bootstrap", nil)
	bootstrap.Header.Set("X-SSHC-Bootstrap", parameters.Get("bootstrap"))
	bootstrap.Header.Set("Origin", engineOrigin)
	bootstrap.Header.Set("Sec-Fetch-Site", "same-origin")
	response, err := client.Do(bootstrap)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return fmt.Errorf("bootstrap failed with status %d", response.StatusCode)
	}
	var session struct {
		CSRFToken string `json:"csrfToken"`
	}
	if err := json.NewDecoder(response.Body).Decode(&session); err != nil {
		return err
	}
	initialise, _ := http.NewRequestWithContext(ctx, http.MethodPost, engineOrigin+"/api/v1/passwords/initialise", bytes.NewBufferString(`{"passphrase":""}`))
	initialise.Header.Set("Content-Type", "application/json")
	initialise.Header.Set("Origin", engineOrigin)
	initialise.Header.Set("Sec-Fetch-Site", "same-origin")
	initialise.Header.Set("X-SSHC-CSRF", session.CSRFToken)
	created, err := client.Do(initialise)
	if err != nil {
		return err
	}
	defer created.Body.Close()
	if created.StatusCode != http.StatusOK {
		return fmt.Errorf("vault initialization failed with status %d", created.StatusCode)
	}
	return nil
}
