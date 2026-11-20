package preset

import (
	"database/sql"
	"encoding/binary"
	"errors"
	"fmt"
	"math"
	"strings"
)

type PersonPhoto struct {
	ID         int64     `json:"id"`
	PersonKey  string    `json:"person_key"`
	FileName   string    `json:"file_name"`
	SourceName string    `json:"source_name"`
	Width      int       `json:"width"`
	Height     int       `json:"height"`
	FaceHeight int       `json:"face_height"`
	Yaw        float64   `json:"yaw"`
	Sharpness  float64   `json:"sharpness"`
	Embedding  []float32 `json:"-"`
	DHash      uint64    `json:"-"`
	AddedAt    string    `json:"added_at"`
}

func encodeEmbedding(v []float32) []byte {
	b := make([]byte, 4*len(v))
	for i, f := range v {
		binary.LittleEndian.PutUint32(b[4*i:], math.Float32bits(f))
	}
	return b
}

func decodeEmbedding(b []byte) ([]float32, error) {
	if len(b)%4 != 0 {
		return nil, fmt.Errorf("эмбеддинг лица: %d байт", len(b))
	}
	v := make([]float32, len(b)/4)
	for i := range v {
		v[i] = math.Float32frombits(binary.LittleEndian.Uint32(b[4*i:]))
	}
	return v, nil
}

const personPhotoCols = `id, person_key, file_name, source_name, width, height, face_height, yaw, sharpness, embedding, dhash, added_at`

func (d *DB) AddPersonPhoto(p *PersonPhoto) error {
	res, err := d.db.Exec(`INSERT INTO person_photos (person_key, file_name, source_name, width, height, face_height, yaw, sharpness, embedding, dhash) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		p.PersonKey, p.FileName, p.SourceName, p.Width, p.Height, p.FaceHeight, p.Yaw, p.Sharpness, encodeEmbedding(p.Embedding), int64(p.DHash))
	if err != nil {
		return err
	}
	p.ID, err = res.LastInsertId()
	if err != nil {
		return err
	}
	return d.db.QueryRow(`SELECT added_at FROM person_photos WHERE id = ?`, p.ID).Scan(&p.AddedAt)
}

func (d *DB) ListPersonPhotos(key string) ([]PersonPhoto, error) {
	q := `SELECT ` + personPhotoCols + ` FROM person_photos`
	args := []any{}
	if key != "" {
		q += ` WHERE person_key = ?`
		args = append(args, key)
	}
	rows, err := d.db.Query(q+` ORDER BY person_key, id`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []PersonPhoto
	for rows.Next() {
		var p PersonPhoto
		var emb []byte
		var dh int64
		if err := rows.Scan(&p.ID, &p.PersonKey, &p.FileName, &p.SourceName, &p.Width, &p.Height, &p.FaceHeight, &p.Yaw, &p.Sharpness, &emb, &dh, &p.AddedAt); err != nil {
			return nil, err
		}
		if p.Embedding, err = decodeEmbedding(emb); err != nil {
			return nil, fmt.Errorf("фото %d: %w", p.ID, err)
		}
		p.DHash = uint64(dh)
		out = append(out, p)
	}
	return out, rows.Err()
}

func (d *DB) GetPersonPhoto(id int64) (*PersonPhoto, error) {
	row := d.db.QueryRow(`SELECT `+personPhotoCols+` FROM person_photos WHERE id = ?`, id)
	var p PersonPhoto
	var emb []byte
	var dh int64
	err := row.Scan(&p.ID, &p.PersonKey, &p.FileName, &p.SourceName, &p.Width, &p.Height, &p.FaceHeight, &p.Yaw, &p.Sharpness, &emb, &dh, &p.AddedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if p.Embedding, err = decodeEmbedding(emb); err != nil {
		return nil, err
	}
	p.DHash = uint64(dh)
	return &p, nil
}

func (d *DB) DeletePersonPhoto(id int64) error {
	_, err := d.db.Exec(`DELETE FROM person_photos WHERE id = ?`, id)
	return err
}

func (d *DB) RenamePersonPhotos(oldKey, newKey string) error {
	keys, err := d.PersonPhotoKeys()
	if err != nil {
		return err
	}
	for _, k := range keys {
		if strings.EqualFold(k, oldKey) {
			if _, err := d.db.Exec(`UPDATE person_photos SET person_key = ? WHERE person_key = ?`, newKey, k); err != nil {
				return err
			}
		}
	}
	return nil
}

func (d *DB) PersonPhotoKeys() ([]string, error) {
	rows, err := d.db.Query(`SELECT DISTINCT person_key FROM person_photos`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var k string
		if err := rows.Scan(&k); err != nil {
			return nil, err
		}
		out = append(out, k)
	}
	return out, rows.Err()
}
