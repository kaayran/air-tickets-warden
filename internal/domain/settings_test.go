package domain

import "testing"

func TestResolveSettings(t *testing.T) {
	def := SettingsDefaults{CooldownHours: 6, DropPct: 0.25, StablePriceBandPct: 0.02}

	t.Run("env defaults when nothing is set", func(t *testing.T) {
		got := ResolveSettings(nil, UserSettings{}, def)
		want := EffectiveSettings{CooldownHours: 6, DropPct: 0.25, StablePriceBandPct: 0.02}
		if got != want {
			t.Errorf("got %+v, want %+v", got, want)
		}
	})

	t.Run("user level overrides env", func(t *testing.T) {
		user := UserSettings{CooldownHours: ptr[int32](12), DropPct: ptr(0.3)}
		got := ResolveSettings(nil, user, def)
		want := EffectiveSettings{CooldownHours: 12, DropPct: 0.3, StablePriceBandPct: 0.02}
		if got != want {
			t.Errorf("got %+v, want %+v", got, want)
		}
	})

	t.Run("subscription level wins over user and env", func(t *testing.T) {
		user := UserSettings{CooldownHours: ptr[int32](12), DropPct: ptr(0.3)}
		sub := &Subscription{CooldownHours: ptr[int32](24), StablePriceBandPct: ptr(0.05)}
		got := ResolveSettings(sub, user, def)
		want := EffectiveSettings{CooldownHours: 24, DropPct: 0.3, StablePriceBandPct: 0.05}
		if got != want {
			t.Errorf("got %+v, want %+v", got, want)
		}
	})

	t.Run("zero at a specific level is a real value, not unset", func(t *testing.T) {
		sub := &Subscription{CooldownHours: ptr[int32](0)}
		got := ResolveSettings(sub, UserSettings{}, def)
		if got.CooldownHours != 0 {
			t.Errorf("explicit 0 cooldown lost: got %d", got.CooldownHours)
		}
	})
}
