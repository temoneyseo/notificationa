package security

import "testing"

func TestEncryptDecryptString(t *testing.T) {
	cipher, err := NewCipher("0123456789abcdef0123456789abcdef")
	if err != nil {
		t.Fatalf("NewCipher: %v", err)
	}

	encrypted, err := cipher.EncryptString("bot-token")
	if err != nil {
		t.Fatalf("EncryptString: %v", err)
	}
	if encrypted == "bot-token" {
		t.Fatal("encrypted value should not equal plaintext")
	}

	decrypted, err := cipher.DecryptString(encrypted)
	if err != nil {
		t.Fatalf("DecryptString: %v", err)
	}
	if decrypted != "bot-token" {
		t.Fatalf("decrypted = %q", decrypted)
	}
}

func TestEncryptSensitiveConfigFields(t *testing.T) {
	cipher, err := NewCipher("0123456789abcdef0123456789abcdef")
	if err != nil {
		t.Fatalf("NewCipher: %v", err)
	}

	config, err := cipher.EncryptConfig(map[string]any{
		"bot_token": "secret",
		"chat_id":   "42",
	})
	if err != nil {
		t.Fatalf("EncryptConfig: %v", err)
	}
	if config["bot_token"] == "secret" {
		t.Fatal("bot_token should be encrypted")
	}
	if config["chat_id"] != "42" {
		t.Fatalf("chat_id should not be encrypted: %+v", config)
	}

	plain, err := cipher.DecryptConfig(config)
	if err != nil {
		t.Fatalf("DecryptConfig: %v", err)
	}
	if plain["bot_token"] != "secret" {
		t.Fatalf("bot_token = %v", plain["bot_token"])
	}
}
