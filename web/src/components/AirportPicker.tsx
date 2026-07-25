import { useEffect, useId, useState } from 'react'
import { useQuery } from '@tanstack/react-query'
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
  label: string
  placeholder: string
  selected: string[]
  max: number
  /** Codes that must not be offered (e.g. the origin, in the destinations picker). */
  exclude?: string[]
  error?: string
  onChange: (next: string[]) => void
}

// AirportPicker is the IATA autocomplete in chart grammar: selected codes render
// as removable chips; focusing an empty field shows the browse list; searching
// starts from the first character. Exposes ARIA combobox/listbox roles so a
// screen reader announces the suggestions.
export function AirportPicker({ label, placeholder, selected, max, exclude = [], error, onChange }: AirportPickerProps) {
  const [query, setQuery] = useState('')
  const [focused, setFocused] = useState(false)
  const debouncedQuery = useDebounced(query.trim(), 250)
  const listId = useId()

  const { data: hits } = useQuery({
    queryKey: ['airports', debouncedQuery],
    queryFn: () => searchAirports(debouncedQuery), // '' -> the browse list
    enabled: focused,
    staleTime: 5 * 60 * 1000, // the dataset is static; cache generously
  })

  const suggestions = (hits ?? []).filter((h) => !selected.includes(h.iata) && !exclude.includes(h.iata))
  const atCapacity = selected.length >= max
  const showList = focused && !atCapacity
  const noMatches = showList && query.trim().length > 0 && suggestions.length === 0

  return (
    <div className="field">
      <span className="field__label">{label}</span>
      {selected.length > 0 && (
        <div className="chips">
          {selected.map((code) => (
            <button
              type="button"
              key={code}
              className="chip"
              onClick={() => onChange(selected.filter((c) => c !== code))}
              aria-label={`Remove ${code}`}
            >
              <span className="data">{code}</span>
              <span className="chip__x" aria-hidden="true">
                ×
              </span>
            </button>
          ))}
        </div>
      )}
      {!atCapacity && (
        <input
          className={`input${error ? ' input--error' : ''}`}
          type="text"
          role="combobox"
          aria-expanded={showList}
          aria-controls={listId}
          aria-autocomplete="list"
          placeholder={placeholder}
          value={query}
          autoCapitalize="characters"
          autoCorrect="off"
          onChange={(e) => setQuery(e.target.value)}
          onFocus={() => setFocused(true)}
          // Delay so a tap on a suggestion lands before the list hides.
          onBlur={() => setTimeout(() => setFocused(false), 150)}
        />
      )}
      {showList && (suggestions.length > 0 || noMatches) && (
        <div className="suggestions" role="listbox" id={listId}>
          {noMatches ? (
            <div className="suggestion__empty">No airport matches “{query.trim()}”.</div>
          ) : (
            suggestions.map((h) => (
              <button
                type="button"
                key={h.iata}
                className="suggestion"
                role="option"
                aria-selected="false"
                onClick={() => {
                  onChange([...selected, h.iata])
                  setQuery('')
                }}
              >
                <span>
                  <span className="suggestion__place">{h.city || h.name}</span>{' '}
                  <span className="suggestion__sub">
                    {h.name} · {h.country}
                  </span>
                </span>
                <span className="suggestion__code">{h.iata}</span>
              </button>
            ))
          )}
        </div>
      )}
      {error && <div className="field__error">{error}</div>}
    </div>
  )
}
