package manager

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"

	"github.com/pocketbase/pocketbase/core"
	"pbmanager/backend/internal/platform"
)

func ensureCollections(app core.App) error {
	for _, name := range []string{"manager_instances", "manager_operations", "manager_activity", "manager_metrics", "manager_agent_tokens", "manager_agent_tasks"} {
		if _, err := app.FindCollectionByNameOrId(name); err == nil {
			continue
		}
		collection := core.NewBaseCollection(name)
		collection.Fields.Add(&core.JSONField{Name: "data", MaxSize: 2 << 20})
		collection.Fields.Add(&core.AutodateField{Name: "created", OnCreate: true}, &core.AutodateField{Name: "updated", OnCreate: true, OnUpdate: true})
		switch name {
		case "manager_agent_tokens":
			collection.Fields.Add(&core.TextField{Name: "hash", Hidden: true, Required: true})
			collection.Indexes = []string{"CREATE UNIQUE INDEX idx_manager_agent_token_hash ON manager_agent_tokens (hash)"}
		case "manager_instances":
			collection.Fields.Add(&core.TextField{Name: "name", Required: true}, &core.NumberField{Name: "port", Required: true, OnlyInt: true})
			collection.Indexes = []string{"CREATE UNIQUE INDEX idx_manager_instance_name ON manager_instances (name)", "CREATE UNIQUE INDEX idx_manager_instance_port ON manager_instances (port)"}
		case "manager_operations":
			collection.Fields.Add(&core.TextField{Name: "instance"}, &core.TextField{Name: "state"}, &core.TextField{Name: "secret", Hidden: true})
		case "manager_metrics":
			collection.Fields.Add(&core.TextField{Name: "instance"}, &core.NumberField{Name: "time", OnlyInt: true}, &core.JSONField{Name: "totals", Hidden: true})
			collection.Indexes = []string{"CREATE INDEX idx_manager_metrics_time ON manager_metrics (time, instance)"}
		}
		// All collection API rules stay locked. Infrastructure writes only use manager routes.
		if err := app.Save(collection); err != nil {
			return fmt.Errorf("create %s: %w", name, err)
		}
	}
	return nil
}

func put(app core.App, collection string, id string, value any, extra map[string]any) (*core.Record, error) {
	var record *core.Record
	var err error
	if id != "" {
		record, err = app.FindRecordById(collection, id)
		if err != nil {
			return nil, err
		}
	} else {
		c, err := app.FindCollectionByNameOrId(collection)
		if err != nil {
			return nil, err
		}
		record = core.NewRecord(c)
	}
	record.Set("data", value)
	for key, value := range extra {
		record.Set(key, value)
	}
	if err = app.Save(record); err != nil {
		return nil, err
	}
	return record, nil
}
func decode[T any](record *core.Record) (T, error) {
	var value T
	err := record.UnmarshalJSONField("data", &value)
	return value, err
}
func instances(app core.App) ([]platform.Instance, error) {
	records, err := app.FindRecordsByFilter("manager_instances", "", "-created", 0, 0)
	if err != nil {
		return nil, err
	}
	result := []platform.Instance{}
	for _, r := range records {
		i, err := decode[platform.Instance](r)
		if err != nil {
			return nil, err
		}
		i.ID = r.Id
		result = append(result, i)
	}
	return result, nil
}
func saveInstance(app core.App, i platform.Instance) error {
	_, err := put(app, "manager_instances", i.ID, i, map[string]any{"name": i.Name, "port": i.Port})
	return err
}
func saveOperation(app core.App, operation platform.Operation) error {
	_, err := put(app, "manager_operations", operation.ID, operation, map[string]any{"instance": operation.InstanceID, "state": operation.State})
	return err
}

func seal(key []byte, value any) (string, error) {
	data, err := json.Marshal(value)
	if err != nil {
		return "", err
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err = io.ReadFull(rand.Reader, nonce); err != nil {
		return "", err
	}
	return base64.StdEncoding.EncodeToString(gcm.Seal(nonce, nonce, data, nil)), nil
}
func unseal(key []byte, value string, result any) error {
	data, err := base64.StdEncoding.DecodeString(value)
	if err != nil {
		return err
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return err
	}
	if len(data) < gcm.NonceSize() {
		return fmt.Errorf("invalid encrypted operation")
	}
	plain, err := gcm.Open(nil, data[:gcm.NonceSize()], data[gcm.NonceSize():], nil)
	if err != nil {
		return err
	}
	return json.Unmarshal(plain, result)
}
