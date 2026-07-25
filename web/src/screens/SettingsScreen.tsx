import { useState } from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { fetchMe, patchMe, type UserSettings } from '../api'
import { Button, Field, FieldGroup, Masthead, Screen, SectionLabel } from '../components/chart/ui'

// numeric form state: '' means "unset — use the service default".
interface SettingsValues {
  cooldown_hours: string
  drop_pct: string // whole percent in the UI; ratio on the wire
  stable_price_band_pct: string
}

function fromSettings(s: UserSettings): SettingsValues {
  return {
    cooldown_hours: s.cooldown_hours != null ? String(s.cooldown_hours) : '',
    drop_pct: s.drop_pct != null ? String(Math.round(s.drop_pct * 100)) : '',
    stable_price_band_pct: s.stable_price_band_pct != null ? String(Math.round(s.stable_price_band_pct * 100)) : '',
  }
}

// toPatch diffs edited values against the loaded settings: only changed keys go
// on the wire; '' maps to null (clear back to the service default).
function toPatch(v: SettingsValues, orig: SettingsValues): Record<string, number | null> {
  const patch: Record<string, number | null> = {}
  if (v.cooldown_hours !== orig.cooldown_hours) {
    patch.cooldown_hours = v.cooldown_hours === '' ? null : Number(v.cooldown_hours)
  }
  if (v.drop_pct !== orig.drop_pct) {
    patch.drop_pct = v.drop_pct === '' ? null : Number(v.drop_pct) / 100
  }
  if (v.stable_price_band_pct !== orig.stable_price_band_pct) {
    patch.stable_price_band_pct = v.stable_price_band_pct === '' ? null : Number(v.stable_price_band_pct) / 100
  }
  return patch
}

// SettingsScreen edits the per-user alert defaults over GET/PATCH /me. These sit
// in the middle of the cascade (a per-watch value wins over them); clearing one
// falls back to the service default. Each control explains what it governs — the
// anti-spam story made legible.
export function SettingsScreen() {
  const queryClient = useQueryClient()
  const { data, error, isLoading } = useQuery({ queryKey: ['me'], queryFn: fetchMe })
  const [edited, setEdited] = useState<SettingsValues | null>(null)
  const [saveError, setSaveError] = useState<string | null>(null)

  const saveMutation = useMutation({
    mutationFn: patchMe,
    onSuccess: (fresh) => {
      queryClient.setQueryData(['me'], fresh)
      setEdited(null)
      setSaveError(null)
    },
    onError: (err) => setSaveError(err instanceof Error ? err.message : 'Request failed'),
  })

  if (isLoading) return <div className="state-msg">Loading settings…</div>
  if (error || !data) {
    return <div className="state-msg">{error instanceof Error ? error.message : 'Could not load settings.'}</div>
  }

  const original = fromSettings(data)
  const values = edited ?? original
  const set = (key: keyof SettingsValues, value: string) => setEdited({ ...values, [key]: value })
  const patch = toPatch(values, original)
  const dirty = Object.keys(patch).length > 0

  return (
    <Screen>
      <Masthead title="Settings" />
      <SectionLabel>Alert defaults</SectionLabel>
      <FieldGroup>
        <Field
          label="Quiet period between alerts (hours)"
          help="After an alert on a route, how long to wait before alerting on it again. Higher means fewer messages."
        >
          <input
            className="input input--data"
            type="number"
            inputMode="numeric"
            min={0}
            placeholder="service default"
            value={values.cooldown_hours}
            onChange={(e) => set('cooldown_hours', e.target.value)}
          />
        </Field>
        <Field
          label="Drop worth alerting (%)"
          help="How far a fare must fall against its history before it counts as a real drop. Higher means only bigger drops alert."
        >
          <input
            className="input input--data"
            type="number"
            inputMode="numeric"
            min={1}
            max={99}
            placeholder="service default"
            value={values.drop_pct}
            onChange={(e) => set('drop_pct', e.target.value)}
          />
        </Field>
        <Field
          label="Ignore wobble within (%)"
          help="Small price moves inside this band are treated as noise, not a drop — this is what keeps repeat alerts quiet."
        >
          <input
            className="input input--data"
            type="number"
            inputMode="numeric"
            min={0}
            max={99}
            placeholder="service default"
            value={values.stable_price_band_pct}
            onChange={(e) => set('stable_price_band_pct', e.target.value)}
          />
        </Field>
      </FieldGroup>
      <p className="await-note">
        Empty fields fall back to the service defaults. A per-watch value always wins over these.
      </p>
      {saveError && (
        <div className="notice" style={{ marginTop: 12 }} role="alert">
          {saveError}
        </div>
      )}
      <div style={{ marginTop: 16 }}>
        <Button variant="primary" block disabled={!dirty || saveMutation.isPending} onClick={() => saveMutation.mutate(patch)}>
          Save
        </Button>
      </div>
    </Screen>
  )
}
