package crypto

/*License: GPLv3
Authors:
Vanilla OS Contributors <https://github.com/vanilla-os/>
Copyright: 2026
Description: LUKS2 token helpers used to mark a device as a Continuity
repository before unlocking it.
*/

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os/exec"
)

// ContinuityTokenType is the LUKS2 token type identifying a Continuity repo.
const ContinuityTokenType = "vanilla-continuity"

// ContinuityToken is stored as a LUKS2 token. The optional fields let callers
// describe the repository without unlocking it.
type ContinuityToken struct {
	Type      string   `json:"type"`
	Keyslots  []string `json:"keyslots"`
	Version   int      `json:"continuity_version"`
	Label     string   `json:"label,omitempty"`
	UUID      string   `json:"uuid,omitempty"`
	CreatedAt string   `json:"created_at,omitempty"`
}

// SetContinuityToken imports a Continuity token, replacing any existing one.
func SetContinuityToken(devicePath string, token ContinuityToken) error {
	if err := RemoveContinuityToken(devicePath); err != nil {
		return err
	}
	token.Type = ContinuityTokenType
	if token.Keyslots == nil {
		token.Keyslots = []string{}
	}
	data, err := json.Marshal(token)
	if err != nil {
		return fmt.Errorf("marshal token: %w", err)
	}
	cmd := exec.Command("cryptsetup", "token", "import", devicePath)
	cmd.Stdin = bytes.NewReader(data)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("luks token import failed: %w\n%s", err, string(out))
	}
	return nil
}

// HasContinuityToken returns the Continuity token from the LUKS2 header.
func HasContinuityToken(devicePath string) (bool, *ContinuityToken, error) {
	tokens, err := listTokens(devicePath)
	if err != nil {
		return false, nil, err
	}
	for _, raw := range tokens {
		var t ContinuityToken
		if err := json.Unmarshal(raw, &t); err != nil {
			continue
		}
		if t.Type == ContinuityTokenType {
			return true, &t, nil
		}
	}
	return false, nil, nil
}

// RemoveContinuityToken deletes any Continuity tokens from the LUKS2 header.
func RemoveContinuityToken(devicePath string) error {
	tokens, err := listTokensIndexed(devicePath)
	if err != nil {
		return err
	}
	for idx, raw := range tokens {
		var t ContinuityToken
		if err := json.Unmarshal(raw, &t); err != nil {
			continue
		}
		if t.Type != ContinuityTokenType {
			continue
		}
		if out, err := exec.Command("cryptsetup", "token", "remove", "--token-id", fmt.Sprintf("%d", idx), devicePath).CombinedOutput(); err != nil {
			return fmt.Errorf("luks token remove %d failed: %w\n%s", idx, err, string(out))
		}
	}
	return nil
}

func listTokens(devicePath string) ([]json.RawMessage, error) {
	idx, err := listTokensIndexed(devicePath)
	if err != nil {
		return nil, err
	}
	out := make([]json.RawMessage, 0, len(idx))
	for _, v := range idx {
		out = append(out, v)
	}
	return out, nil
}

func listTokensIndexed(devicePath string) (map[int]json.RawMessage, error) {
	cmd := exec.Command("cryptsetup", "luksDump", "--dump-json-metadata", devicePath)
	out, err := cmd.Output()
	if err != nil {
		// Older cryptsetup lacks --dump-json-metadata; treat as no tokens.
		return map[int]json.RawMessage{}, nil
	}
	var dump struct {
		Tokens map[string]json.RawMessage `json:"tokens"`
	}
	if err := json.Unmarshal(out, &dump); err != nil {
		return nil, fmt.Errorf("parse luksDump: %w", err)
	}
	result := make(map[int]json.RawMessage, len(dump.Tokens))
	for k, v := range dump.Tokens {
		var idx int
		if _, err := fmt.Sscanf(k, "%d", &idx); err != nil {
			continue
		}
		result[idx] = v
	}
	return result, nil
}
