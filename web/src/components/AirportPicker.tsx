import { useEffect, useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { Cell, Chip, Input, Section } from '@telegram-apps/telegram-ui'
import { searchAirports } from '../api'

// useDebounced delays the autocomplete query so we don't hit the API on every
// keystroke.
function useDebounced(value: string, ms: number): string {
  const [debounced, setDebounced] = useState(value)
  useEffect(() => {
    const t = setTimeout(() => setDebounced(value), ms)
    return () => clearTimeout(t)
  }, [value, ms])
  return debounced
}

export interface AirportPickerProps {
  header: string
  placeholder: string
  selected: string[]
  max: number
  /** Codes that must not be offered (e.g. the origin, in the destinations picker). */
  exclude?: string[]
  error?: string
  onChange: (next: string[]) => void
}

// AirportPicker is the IATA autocomplete: selected codes render as removable
// chips; typing 2+ characters searches /api/v1/airports.
export function AirportPicker({ header, placeholder, selected, max, exclude = [], error, onChange }: AirportPickerProps) {
  const [query, setQuery] = useState('')
  const debouncedQuery = useDebounced(query.trim(), 250)

  const { data: hits } = useQuery({
    queryKey: ['airports', debouncedQuery],
    queryFn: () => searchAirports(debouncedQuery),
    enabled: debouncedQuery.length >= 2,
    staleTime: 5 * 60 * 1000, // the dataset is static; cache generously
  })

  const suggestions = (hits ?? []).filter((h) => !selected.includes(h.iata) && !exclude.includes(h.iata))
  const atCapacity = selected.length >= max

  return (
    <Section header={header} footer={error}>
      {selected.length > 0 && (
        <div style={{ display: 'flex', flexWrap: 'wrap', gap: 8, padding: '8px 16px' }}>
          {selected.map((code) => (
            <Chip key={code} mode="mono" after="✕" onClick={() => onChange(selected.filter((c) => c !== code))}>
              {code}
            </Chip>
          ))}
        </div>
      )}
      {!atCapacity && (
        <Input
          placeholder={placeholder}
          value={query}
          status={error ? 'error' : undefined}
          autoCapitalize="none"
          autoCorrect="off"
          onChange={(e) => setQuery(e.target.value)}
        />
      )}
      {!atCapacity &&
        query.trim().length >= 2 &&
        suggestions.map((h) => (
          <Cell
            key={h.iata}
            subtitle={`${h.name} · ${h.country}`}
            after={h.iata}
            onClick={() => {
              onChange([...selected, h.iata])
              setQuery('')
            }}
          >
            {h.city || h.name}
          </Cell>
        ))}
    </Section>
  )
}
