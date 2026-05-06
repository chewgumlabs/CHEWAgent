# Gum Keys

Gum Keys are small public or private profiles for CHEWAgent. They keep the
same CHEW shell and change the Gum layer: what project facts matter, how
status should be phrased, and whether a richer status provider is available.

## Public Gum

[`public.gum-key.json`](public.gum-key.json) is the portable public profile.
CHEW loads the same profile by default even when `CHEW_GUM_KEY` is unset. This
JSON file is the publishable/template copy. It has no private endpoints, no
workspace assumptions, and no status command. It only adds lightweight guidance
to the brain prompt:

- CHEW stays the conversational partner.
- Gum is the quiet project spine.
- Progress reports stay human, but traceable with `Checkpoint`, `Next`, and
  `Blocked` labels when useful.
- Users should chat about intent; they should not be asked to edit `GUM.md`.

Point at the explicit file from this checkout:

```sh
CHEW_GUM_KEY="$PWD/gum-keys/public.gum-key.json" chew
```

The default public install works without that env var. This file exists as a
concrete template for people who want the lightweight Gum workflow explicitly,
and as the shape private/internal Gum packs can build from.
