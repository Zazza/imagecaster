package mcp

import (
	"fmt"
	"strings"

	"github.com/Zazza/imagecaster/internal/generation"
	"github.com/Zazza/imagecaster/internal/people"
	"github.com/Zazza/imagecaster/internal/serverclient"
)

var modelBaseOf = generation.ModelBase

func personKeyFor(list []serverclient.Person, name string) (string, bool) {
	return people.Find(list, name)
}

func personNames(list []serverclient.Person) string {
	seen := map[string]bool{}
	var out []string
	for _, p := range list {
		if k := people.Key(p); !seen[k] {
			seen[k] = true
			out = append(out, k)
		}
	}
	return strings.Join(out, ", ")
}

func resolvePerson(list []serverclient.Person, name string, models []string, faceSwap bool) (*generation.PersonOverride, error) {
	key, ok := personKeyFor(list, name)
	if !ok {
		return nil, fmt.Errorf("человек %q не найден; есть: %s", name, personNames(list))
	}
	all := people.LorasByBase(list, key)
	loras := map[string]string{}
	var bases []string
	first := ""
	for _, m := range models {
		b := modelBaseOf(m)
		if l, ok := all[b]; ok && b != "" {
			if _, dup := loras[b]; !dup {
				bases = append(bases, b)
			}
			loras[b] = l
			if first == "" {
				first = l
			}
		}
	}
	if first == "" {
		have := make([]string, 0, len(all))
		for b := range all {
			have = append(have, b)
		}
		return nil, fmt.Errorf("«%s»: нет LoRA для моделей этой генерации; LoRA есть для: %s", key, strings.Join(have, ", "))
	}
	ov := &generation.PersonOverride{Name: first, Loras: loras}
	if faceSwap {
		if n := people.FaceSwapName(list, key, bases); n != "" {
			ov.FaceSwap, ov.FaceSwapName = true, n
		}
	}
	return ov, nil
}

func (s *Server) personList() ([]serverclient.Person, error) {
	if s.deps.People == nil {
		return nil, fmt.Errorf("список людей недоступен: сервер Imagecaster не настроен")
	}
	return allPeople(s.deps.People)
}

func (s *Server) compoundPerson(who string, gp generation.GenerateCompoundImageParams, faceSwap bool) (*generation.PersonOverride, error) {
	var ids []int64
	if len(gp.Steps) > 0 {
		for _, st := range gp.Steps {
			ids = append(ids, st.PresetID)
		}
	} else {
		cp, err := s.deps.DB.GetCompoundPresetFull(gp.CompoundPresetID)
		if err != nil {
			return nil, fmt.Errorf("compound preset not found: %w", err)
		}
		for _, st := range cp.Steps {
			ids = append(ids, st.PresetID)
		}
	}
	models := make([]string, 0, len(ids))
	for _, id := range ids {
		p, err := s.deps.DB.Get(id)
		if err != nil {
			return nil, fmt.Errorf("preset %d not found: %w", id, err)
		}
		models = append(models, p.ModelName)
	}
	list, err := s.personList()
	if err != nil {
		return nil, err
	}
	return resolvePerson(list, who, models, faceSwap)
}

type pipelineLookup func(id int64) (string, []string, error)

func (s *Server) pipelineModels(id int64) (string, []string, error) {
	if s.deps.DB == nil {
		return "", nil, fmt.Errorf("база пресетов недоступна")
	}
	name, models, err := s.deps.DB.CompoundStepModels(id)
	if err != nil {
		return "", nil, fmt.Errorf("пайплайн %d: %w", id, err)
	}
	return name, models, nil
}

func (s *Server) personPipelines(list []serverclient.Person, key string) []map[string]any {
	out := []map[string]any{}
	if s.deps.DB == nil {
		return out
	}
	cps, err := s.deps.DB.ListCompoundPresetsFull()
	if err != nil {
		return out
	}
	have := people.LorasByBase(list, key)
	for _, cp := range cps {
		_, models, err := s.pipelineModels(cp.ID)
		if err != nil {
			continue
		}
		with := 0
		for _, m := range models {
			if _, ok := have[modelBaseOf(m)]; ok {
				with++
			}
		}
		if with > 0 {
			out = append(out, map[string]any{"id": cp.ID, "name": cp.Name, "steps": len(models), "steps_with_lora": with})
		}
	}
	return out
}
