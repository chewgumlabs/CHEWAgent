# CHEW Entrypoints

CHEWAgent is one shell with two separable attachments:

- **Gum Key**: project guidance, workflow language, status/orientation shape.
- **Runtime profile**: model endpoint, state directory, launcher-specific roots.

That split lets public and private builds feed the same CHEW without forking the
mascot, TUI, planner, or tool layer.

## Public `chew`

`chew` is the normal public command. If `CHEW_GUM_KEY` is unset, CHEW loads the
built-in Public Gum profile. Users do not need to know a key exists.

Public defaults:

- Gum Key: built-in `Public Gum`
- state home: default CHEWAgent brain directory
- brain: Bonsai through `install brain` / `wake up`
- status provider: local `.gum/status.json` trail only

The publishable copy of the public key lives at
[`gum-keys/public.gum-key.json`](../gum-keys/public.gum-key.json). The built-in
copy is tested against that file so release binaries do not depend on the source
checkout being present.

## Private Or Internal Launchers

Private launchers should keep using the same `chew` binary and change only env:

```sh
CHEW_GUM_KEY=/path/to/internal.gum-key.json \
CHEW_STATE_HOME=/path/to/private/state \
chew
```

Optional launcher env can point the key's `status_command` at richer workflow
machinery, but the TUI still receives the same kind of answer: human status
text for CHEW to show.
When that machinery has an active workflow marker, it should conform to
[`gum-keys/gum-orientation.schema.json`](../gum-keys/gum-orientation.schema.json)
and be translated into prose before it reaches the user.

The intended pattern is:

| command | shell | Gum Key | runtime profile |
|---|---|---|---|
| `chew` | public CHEWAgent | built-in Public Gum | local Bonsai |
| `CHEW_GUM_KEY=... chew` | public CHEWAgent | custom/public pack | caller's runtime |
| `chew-internal` | public CHEWAgent | private SwarmLab Gum | private state + model endpoint |

## Boundary

Gum Keys should not choose models. Runtime profiles should not define workflow
truth. Keeping that boundary clear lets public improvements and private Gum
packs share the same core CHEW behavior without stepping on each other.

## Hidden OpenAI-Compatible Brains

Public CHEWAgent still defaults to Bonsai. Stronger hosted models can be added
as hidden runtime profiles in CHEW's state-local `brain/config.json`; API keys
stay in environment variables and are never written to the profile file.

Example DeepSeek V4 profile:

```json
{
  "name": "deepseek-v4-pro",
  "display_name": "DeepSeek V4 Pro",
  "provider": "openai-compatible",
  "source": "private",
  "managed": false,
  "base_url": "https://api.deepseek.com",
  "chat_path": "/chat/completions",
  "model_alias": "deepseek-v4-pro",
  "api_key_env": "DEEPSEEK_API_KEY",
  "extra_body": {
    "reasoning_effort": "high",
    "thinking": { "type": "enabled" }
  }
}
```

With that profile present:

```sh
export DEEPSEEK_API_KEY="..."
chew
# inside CHEW:
# profile use deepseek-v4-pro
# wake up
```

The same profile shape works for OpenAI-compatible providers by changing
`base_url`, `chat_path`, `model_alias`, and `api_key_env`. If `chat_path` is
omitted, CHEW uses `/v1/chat/completions`, which matches local `llama-server`
and OpenAI-style endpoints.
