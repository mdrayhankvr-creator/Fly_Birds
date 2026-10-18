package school

import (
	"context"
	"encoding/json"
	"errors"
	"slices"
	"time"

	"github.com/mdrayhankvr-creator/Fly_Birds/backend/internal/audit"
	"gorm.io/gorm"
)

// ErrDependencies marks a module change blocked by its dependency graph.
var ErrDependencies = errors.New("module dependencies not satisfied")

// DependencyError names the modules blocking a change. Kind is "requires"
// when enabling needs disabled prerequisites and "required_by" when disabling
// would break enabled modules that depend on it.
type DependencyError struct {
	Kind    string
	Modules []string
}

func (e *DependencyError) Error() string        { return ErrDependencies.Error() }
func (e *DependencyError) Is(target error) bool { return target == ErrDependencies }

type moduleState struct {
	IsCore    bool
	Enabled   bool
	DependsOn []string
}

// PlanModuleChange returns the modules to switch, in a safe order, so that the
// dependency graph stays satisfied. Without cascade it refuses any change that
// would also need other modules to switch.
func PlanModuleChange(all map[string]moduleState, key string, enabled, cascade bool) ([]string, error) {
	target, ok := all[key]
	if !ok {
		return nil, ErrNotFound
	}
	if target.Enabled == enabled {
		return []string{}, nil
	}
	if !enabled && target.IsCore {
		return nil, ErrCoreModule
	}
	var order []string
	seen := map[string]bool{}
	if enabled {
		// Prerequisites first, depth first, so each enabled module's needs hold.
		var visit func(string)
		visit = func(k string) {
			if seen[k] {
				return
			}
			seen[k] = true
			for _, dep := range all[k].DependsOn {
				visit(dep)
			}
			if k != key && !all[k].Enabled {
				order = append(order, k)
			}
		}
		visit(key)
		if len(order) > 0 && !cascade {
			return nil, &DependencyError{Kind: "requires", Modules: order}
		}
		return append(order, key), nil
	}
	// Dependents first, so nothing enabled is left without a prerequisite.
	var visit func(string)
	visit = func(k string) {
		if seen[k] {
			return
		}
		seen[k] = true
		keys := make([]string, 0, len(all))
		for other := range all {
			keys = append(keys, other)
		}
		slices.Sort(keys)
		for _, other := range keys {
			if all[other].Enabled && slices.Contains(all[other].DependsOn, k) {
				visit(other)
			}
		}
		if k != key {
			order = append(order, k)
		}
	}
	visit(key)
	if len(order) > 0 && !cascade {
		return nil, &DependencyError{Kind: "required_by", Modules: order}
	}
	return append(order, key), nil
}

const moduleSelect = "d.key,d.name,d.is_core,m.enabled,array_to_json(d.depends_on)::text AS depends_json,to_char(m.updated_at AT TIME ZONE 'UTC','YYYY-MM-DD\"T\"HH24:MI:SS\"Z\"') AS updated_at"

func moduleBase(db *gorm.DB) *gorm.DB {
	return db.Table("module_definitions AS d").Joins("JOIN school_modules AS m ON m.module_key = d.key AND m.school_id = ?", SingleSchoolID)
}

// allModules loads the whole (small) module table so dependents can be shown
// and plans checked against every module.
func allModules(db *gorm.DB) ([]Module, error) {
	rows := []Module{}
	if err := moduleBase(db).Select(moduleSelect).Order("d.key").Scan(&rows).Error; err != nil {
		return nil, err
	}
	for i := range rows {
		rows[i].DependsOn = []string{}
		_ = json.Unmarshal([]byte(rows[i].DependsJSON), &rows[i].DependsOn)
	}
	for i := range rows {
		rows[i].Dependents = []string{}
		for _, other := range rows {
			if slices.Contains(other.DependsOn, rows[i].Key) {
				rows[i].Dependents = append(rows[i].Dependents, other.Key)
			}
		}
	}
	return rows, nil
}

func (r *GormRepository) Modules(ctx context.Context, search string, page, size int) (ModulePage, error) {
	rows, err := allModules(r.db.WithContext(ctx))
	if err != nil {
		return ModulePage{}, err
	}
	items := []Module{}
	for _, m := range rows {
		if search == "" || containsFold(m.Key, search) || containsFold(m.Name, search) {
			items = append(items, m)
		}
	}
	total := int64(len(items))
	start := min((page-1)*size, len(items))
	end := min(start+size, len(items))
	return ModulePage{Items: items[start:end], Total: total, Page: page, PageSize: size}, nil
}

func moduleIn(tx *gorm.DB, key string) (Module, error) {
	var module Module
	result := moduleBase(tx).Select(moduleSelect).Where("d.key = ?", key).Take(&module)
	if errors.Is(result.Error, gorm.ErrRecordNotFound) {
		return Module{}, ErrNotFound
	}
	return module, result.Error
}

func (r *GormRepository) ModuleEnabled(ctx context.Context, key string) (bool, error) {
	module, err := moduleIn(r.db.WithContext(ctx), key)
	return module.Enabled, err
}

// UpdateModule applies a dependency-checked plan in one transaction and
// audits every module it switches.
func (r *GormRepository) UpdateModule(ctx context.Context, actor, key string, enabled, cascade bool) (Module, error) {
	var updated Module
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := schoolLock(tx); err != nil {
			return err
		}
		if err := schoolGuard(tx, actor, "modules:manage"); err != nil {
			return err
		}
		rows, err := allModules(tx)
		if err != nil {
			return err
		}
		states := map[string]moduleState{}
		for _, m := range rows {
			states[m.Key] = moduleState{IsCore: m.IsCore, Enabled: m.Enabled, DependsOn: m.DependsOn}
		}
		plan, err := PlanModuleChange(states, key, enabled, cascade)
		if err != nil {
			return err
		}
		for _, k := range plan {
			if err := tx.Table("school_modules").Where("school_id = ? AND module_key = ?", SingleSchoolID, k).Updates(map[string]any{"enabled": enabled, "updated_at": time.Now().UTC()}).Error; err != nil {
				return err
			}
			meta := map[string]any{"before_enabled": !enabled, "after_enabled": enabled}
			if k != key {
				meta["via"] = key
			}
			if err := audit.Record(tx, &actor, "school.module_updated", "module", k, meta); err != nil {
				return err
			}
		}
		updated, err = moduleIn(tx, key)
		if err == nil {
			updated.DependsOn = states[key].DependsOn
			for _, m := range rows {
				if m.Key == key {
					updated.Dependents = m.Dependents
				}
			}
			updated.Changed = plan
		}
		return err
	})
	return updated, err
}
