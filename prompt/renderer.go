package prompt

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/ygpkg/yg-go/prompt/model"
	"gorm.io/gorm"
)

type Renderer struct {
	db *gorm.DB
}

func NewRenderer(db *gorm.DB) *Renderer {
	return &Renderer{db: db}
}

func (r *Renderer) RenderByCode(ctx context.Context, code string, values map[string]any) (string, error) {
	promptDao := model.NewPromptDao(r.db)
	prompt, err := promptDao.GetByCode(ctx, code)
	if err != nil {
		return "", fmt.Errorf("get prompt by code %q fail: %w", code, err)
	}
	if prompt.ID == 0 {
		return "", fmt.Errorf("%w: code=%s", model.ErrPromptNotFound, code)
	}
	if prompt.Status == model.PromptStatusDisabled {
		return "", fmt.Errorf("%w: code=%s, promptID=%d", model.ErrPromptDisabled, code, prompt.ID)
	}

	verDao := model.NewPromptVersionDao(r.db)
	if prompt.LatestVersionID == 0 {
		return "", fmt.Errorf("%w: promptID=%d, latestVersionID=0", model.ErrVersionNotFound, prompt.ID)
	}
	version, err := verDao.GetByID(ctx, prompt.LatestVersionID)
	if err != nil {
		return "", fmt.Errorf("get prompt version %d fail: %w", prompt.LatestVersionID, err)
	}
	if version.ID == 0 {
		return "", fmt.Errorf("%w: promptID=%d, versionID=%d", model.ErrVersionNotFound, prompt.ID, prompt.LatestVersionID)
	}

	var keys []model.VarKey
	if err := json.Unmarshal(version.VariableKeys, &keys); err != nil {
		return "", fmt.Errorf("unmarshal variable_keys fail: promptID=%d, versionID=%d, err: %w", prompt.ID, version.ID, err)
	}

	return model.ValidateAndRender(ctx, version.Content, keys, values)
}
