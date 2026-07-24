package domain

// UserSettings are the per-user alert defaults (the middle tier of the
// cascade). Nil means "not set at this level".
type UserSettings struct {
	ChatID             int64
	CooldownHours      *int32
	DropPct            *float64
	StablePriceBandPct *float64
}

// SettingsDefaults is the env-level tail of the cascade — always fully
// populated (config supplies hard defaults).
type SettingsDefaults struct {
	CooldownHours      int32
	DropPct            float64
	StablePriceBandPct float64
}

// EffectiveSettings is a fully-resolved parameter set: every field has a
// concrete value after the cascade.
type EffectiveSettings struct {
	CooldownHours      int32
	DropPct            float64
	StablePriceBandPct float64
}

// ResolveSettings applies the cascade subscription → user settings → env
// defaults: the most specific level that sets a value wins. sub may be nil
// (user-level resolution, e.g. for the settings screen preview).
func ResolveSettings(sub *Subscription, user UserSettings, def SettingsDefaults) EffectiveSettings {
	eff := EffectiveSettings(def)
	if user.CooldownHours != nil {
		eff.CooldownHours = *user.CooldownHours
	}
	if user.DropPct != nil {
		eff.DropPct = *user.DropPct
	}
	if user.StablePriceBandPct != nil {
		eff.StablePriceBandPct = *user.StablePriceBandPct
	}
	if sub != nil {
		if sub.CooldownHours != nil {
			eff.CooldownHours = *sub.CooldownHours
		}
		if sub.DropPct != nil {
			eff.DropPct = *sub.DropPct
		}
		if sub.StablePriceBandPct != nil {
			eff.StablePriceBandPct = *sub.StablePriceBandPct
		}
	}
	return eff
}
