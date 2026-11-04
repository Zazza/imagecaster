package preset

import (
	"encoding/json"
	"fmt"
	"strings"
)

const (
	MaxRecipeNameLen   = 100
	MaxRecipeStepsLen  = 8192
	MaxRecipeAdjustLen = 8192
)

type PostRecipe struct {
	ID        int64  `json:"id"`
	Name      string `json:"name"`
	Steps     string `json:"steps"`
	Adjust    string `json:"adjust"`
	CreatedAt string `json:"created_at"`
	UpdatedAt string `json:"updated_at"`
}

const postRecipeColumns = `id, name, steps, adjust, created_at, updated_at`

func (d *DB) ListPostRecipes() ([]PostRecipe, error) {
	rows, err := d.db.Query(`SELECT ` + postRecipeColumns + ` FROM post_recipes ORDER BY id ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var items []PostRecipe
	for rows.Next() {
		var r PostRecipe
		if err := rows.Scan(&r.ID, &r.Name, &r.Steps, &r.Adjust, &r.CreatedAt, &r.UpdatedAt); err != nil {
			return nil, err
		}
		items = append(items, r)
	}
	return items, rows.Err()
}

func (d *DB) SavePostRecipe(r PostRecipe) (*PostRecipe, error) {
	r.Name = strings.TrimSpace(r.Name)
	if r.Name == "" {
		return nil, fmt.Errorf("recipe name is required")
	}
	if len(r.Name) > MaxRecipeNameLen {
		return nil, fmt.Errorf("recipe name too long (max %d chars)", MaxRecipeNameLen)
	}
	if r.Steps == "" || len(r.Steps) > MaxRecipeStepsLen || !json.Valid([]byte(r.Steps)) {
		return nil, fmt.Errorf("recipe steps must be valid JSON (max %d bytes)", MaxRecipeStepsLen)
	}
	if r.Adjust == "" || len(r.Adjust) > MaxRecipeAdjustLen || !json.Valid([]byte(r.Adjust)) {
		return nil, fmt.Errorf("recipe adjust must be valid JSON (max %d bytes)", MaxRecipeAdjustLen)
	}

	if r.ID == 0 {
		result, err := d.db.Exec(
			`INSERT INTO post_recipes (name, steps, adjust) VALUES (?, ?, ?)`,
			r.Name, r.Steps, r.Adjust,
		)
		if err != nil {
			return nil, err
		}
		r.ID, _ = result.LastInsertId()
		return &r, nil
	}

	result, err := d.db.Exec(
		`UPDATE post_recipes SET name=?, steps=?, adjust=?, updated_at=datetime('now') WHERE id=?`,
		r.Name, r.Steps, r.Adjust, r.ID,
	)
	if err != nil {
		return nil, err
	}
	if n, _ := result.RowsAffected(); n == 0 {
		return nil, fmt.Errorf("post recipe %d not found", r.ID)
	}
	return &r, nil
}

func (d *DB) DeletePostRecipe(id int64) error {
	_, err := d.db.Exec(`DELETE FROM post_recipes WHERE id = ?`, id)
	return err
}
