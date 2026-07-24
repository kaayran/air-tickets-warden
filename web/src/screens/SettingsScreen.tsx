import { useState } from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { Button, Input, List, Placeholder, Section, Spinner } from '@telegram-apps/telegram-ui'
import { fetchMe, patchMe, type UserSettings } from '../api'

// numeric form state: '' means "unset — use the default from the cascade".
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

// toPatch diffs the edited values against the loaded settings: only changed
// keys go on the wire; '' maps to null (clear back to the env default).
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

// SettingsScreen edits the per-user alert defaults over GET/PATCH /me. These
// sit in the middle of the cascade: a subscription-level value wins over
// them, and clearing one falls back to the service default.
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

  if (isLoading) {
    return (
      <Placeholder>
        <Spinner size="l" />
      </Placeholder>
    )
  }
  if (error || !data) {
    return (
      <Placeholder
        header="Could not load settings"
        description={error instanceof Error ? error.message : 'Unknown error'}
      />
    )
  }

  const original = fromSettings(data)
  const values = edited ?? original
  const set = (key: keyof SettingsValues, value: string) =>
    setEdited({ ...values, [key]: value })
  const patch = toPatch(values, original)
  const dirty = Object.keys(patch).length > 0

  return (
    <List>
      <Section
        header="Alert defaults"
        footer="Empty fields fall back to the service defaults. A per-subscription value always wins over these."
      >
        <Input
          header="Cooldown between alerts (hours)"
          type="number"
          inputMode="numeric"
          min={0}
          placeholder="service default"
          value={values.cooldown_hours}
          onChange={(e) => set('cooldown_hours', e.target.value)}
        />
        <Input
          header="Price drop worth alerting (%)"
          type="number"
          inputMode="numeric"
          min={1}
          max={99}
          placeholder="service default"
          value={values.drop_pct}
          onChange={(e) => set('drop_pct', e.target.value)}
        />
        <Input
          header="Stable-price band (%)"
          type="number"
          inputMode="numeric"
          min={0}
          max={99}
          placeholder="service default"
          value={values.stable_price_band_pct}
          onChange={(e) => set('stable_price_band_pct', e.target.value)}
        />
      </Section>
      {saveError && <Section footer={saveError} />}
      <Section footer={`Signed in as chat ${data.chat_id}`}>
        <div style={{ padding: 16 }}>
          <Button
            size="l"
            stretched
            disabled={!dirty}
            loading={saveMutation.isPending}
            onClick={() => saveMutation.mutate(patch)}
          >
            Save
          </Button>
        </div>
      </Section>
    </List>
  )
}
