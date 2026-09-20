# LLM diagnosis setup

This guide configures `rotari diagnose` with OpenAI, Anthropic, Gemini, or an
OpenAI-compatible endpoint.

## 1. Create an API key

Create or select an OpenAI project, then create a secret API key in the
[OpenAI API key page](https://platform.openai.com/api-keys). The project must
have API access. API billing and ChatGPT subscriptions are separate. Add API
credits in [OpenAI billing settings](https://platform.openai.com/settings/organization/billing/)
before the first request.

Copy the key when it is displayed. Do not commit it, put it in a rotari config
file, or add it to a job with `rotari add --env`.

If `diagnose` reports `429 Too Many Requests` with `insufficient_quota` or
`credit_balance_exhausted`, the OpenAI API project has no usable credits.
Open the billing settings link above, add credits or correct the project's
billing configuration, then run the command again.

## 2. Set the key for one shell session

Build the checkout, then export the key and a model name in the terminal where
you will run the diagnosis:

```sh
go build -o rotari ./cmd/rotari
export ROTARI_LLM_API_KEY='paste-the-key-here'
export ROTARI_LLM_MODEL='gpt-5-mini'
```

`ROTARI_LLM_API_KEY` exists only in that shell and its child processes. Do not
add it to shell startup files unless you understand the security implications.
Use `unset ROTARI_LLM_API_KEY` when finished.

## 3. Run the sample

```sh
./scripts/example-diagnose.sh
```

The sample creates an isolated temporary rotari state directory, runs a Python
job that intentionally fails with `ModuleNotFoundError`, and asks the model to
diagnose that log. It prints the state directory afterward for inspection.

## 4. Diagnose your own job

First inspect the log, then send one job explicitly:

```sh
rotari show --run-id RUN_ID --job-id JOB_ID
rotari diagnose --run-id RUN_ID --job-id JOB_ID --model "$ROTARI_LLM_MODEL" --language ja
```

The request contains the job's command, recorded exit/error information, and
at most the final 12,000 characters of its output log. It does not rerun the
job. rotari does not persist the API key or the returned diagnosis.

`--language` accepts a BCP 47 language tag such as `ja`, `en`, or `en-US` and
asks the model to answer in that language. Set `ROTARI_LLM_LANGUAGE=ja` to use
the same default for multiple commands. When neither is set, the model chooses
its response language.

## Endpoint and model examples

`diagnose` supports OpenAI Responses API, OpenAI-compatible Chat Completions,
Anthropic Messages API, and Gemini `generateContent` endpoints.

The supported connection choices are equivalent at the rotari boundary:

| Provider | Compute location | Endpoint | Authentication | Billing |
| --- | --- | --- | --- | --- |
| OpenAI | OpenAI cloud | OpenAI Responses API | OpenAI API key | API usage billing |
| OpenAI-compatible | Provider cloud or local | Chat Completions API | Bearer API key | Provider billing |
| Ollama Cloud | Ollama cloud | Ollama Responses API | Ollama API key | Free starter usage, then credits or a paid plan |
| Ollama Local | Your machine | Local Ollama server | Placeholder value; ignored by default | No model usage charge; local hardware and power are yours |
| Claude | Anthropic cloud | Anthropic Messages API | Anthropic API key | Anthropic API usage billing |
| Gemini | Google cloud | Gemini `generateContent` API | Gemini API key | Google API usage billing |
| Cohere | Cohere cloud | Cohere v2 Chat API | Cohere API key | Cohere API usage billing |

All seven choices can be used directly. Select Claude with
`ROTARI_LLM_PROVIDER=anthropic`; select OpenAI or an OpenAI-compatible
endpoint with `ROTARI_LLM_PROVIDER=openai-chat`, or Gemini with
`ROTARI_LLM_PROVIDER=gemini`. The `openai` provider is the default.

### OpenAI

This is the default provider. Before the first request:

1. Create or select an OpenAI project.
2. Create a secret key at the [OpenAI API key page](https://platform.openai.com/api-keys).
3. Add API credits in [OpenAI billing settings](https://platform.openai.com/settings/organization/billing/).

The API billing account is separate from any ChatGPT subscription. Do not
commit the key or put it in a rotari config file.

`gpt-5-mini` is a practical first choice for short log diagnoses; use `gpt-5`
when a more thorough diagnosis is worth the additional cost and latency.

Model candidates:

- `gpt-5-mini`: first choice for lower cost and latency.
- `gpt-5`: more thorough diagnosis when cost and latency are less important.
- `gpt-4.1-mini` and `gpt-4.1`: alternatives when they are enabled for the
	project.

```sh
export ROTARI_LLM_API_KEY='your-openai-api-key'
export ROTARI_LLM_ENDPOINT='https://api.openai.com/v1/responses'
export ROTARI_LLM_MODEL='gpt-5-mini'
```

### OpenAI-compatible Chat Completions

Use `openai-chat` for providers that expose the OpenAI Chat Completions
format. The request uses `POST /v1/chat/completions`, a Bearer API key, and a
`messages` array. The endpoint and model name come from the provider.

Supported examples include Mistral, Groq, DeepSeek, and xAI Grok:

| Provider | Endpoint example | Model example |
| --- | --- | --- |
| Mistral | `https://api.mistral.ai/v1/chat/completions` | `mistral-small-latest` |
| Groq | `https://api.groq.com/openai/v1/chat/completions` | Provider catalog |
| DeepSeek | `https://api.deepseek.com/chat/completions` | `deepseek-chat` |
| xAI Grok | `https://api.x.ai/v1/chat/completions` | Provider catalog |

For example, to use DeepSeek:

```sh
export ROTARI_LLM_PROVIDER='openai-chat'
export ROTARI_LLM_ENDPOINT='https://api.deepseek.com/chat/completions'
export ROTARI_LLM_API_KEY='your-deepseek-api-key'
export ROTARI_LLM_MODEL='deepseek-chat'
```

Mistral, Groq, and xAI use the same variables; replace the endpoint, API key,
and model with values from the selected provider. Cohere uses a different API
format and is not covered by `openai-chat`.

### Ollama on the local machine

[Ollama's OpenAI compatibility API](https://docs.ollama.com/api/openai-compatibility)
supports `POST /v1/responses` from version 0.13.3. Download a model first,
then point rotari at the local server. The API key value is required by rotari
but ignored by a default local Ollama server.

Model candidates:

- `qwen3:8b`: a moderate-size first choice for a local diagnosis model.
- `gpt-oss:20b`: a larger choice when the host has sufficient memory.
- `llama3.2`: use this when it is the installed local model.

```sh
ollama pull qwen3:8b
export ROTARI_LLM_ENDPOINT='http://localhost:11434/v1/responses'
export ROTARI_LLM_API_KEY='ollama'
export ROTARI_LLM_MODEL='qwen3:8b'
```

### Ollama cloud

Ollama exposes a hosted Responses API. Before the first request:

1. Open [ollama.com](https://ollama.com/) and create an account or sign in.
2. Open [API key settings](https://ollama.com/settings/keys), create an API
	key, and copy it when shown.
3. Choose a model from the [Ollama cloud catalog](https://ollama.com/search?c=cloud).
4. Set the endpoint, copied key, and selected model name in the shell:

```sh
export ROTARI_LLM_ENDPOINT='https://ollama.com/v1/responses'
export ROTARI_LLM_API_KEY='your-ollama-api-key'
export ROTARI_LLM_MODEL='your-ollama-cloud-model'
```

API keys do not expire; revoke a key from the same API key settings page if it
is exposed.

Model availability and pricing change by provider, so check its model catalog
before choosing a production default.

### Claude

Create an API key in the [Anthropic Console](https://console.anthropic.com/),
add API credits, and choose a Claude model there. A Claude subscription does
not provide an Anthropic API key.

Model candidates:

- `claude-sonnet-5`: a balanced first choice for log diagnosis.
- `claude-opus-5`: a more capable choice when thoroughness matters more than
	cost and latency.
- `claude-haiku-4-5`: a faster, lower-cost choice for straightforward logs.

```sh
export ROTARI_LLM_PROVIDER='anthropic'
export ROTARI_LLM_ENDPOINT='https://api.anthropic.com/v1/messages'
export ROTARI_LLM_API_KEY='your-anthropic-api-key'
export ROTARI_LLM_MODEL='claude-sonnet-5'
```

### Gemini

Create a Gemini API key in [Google AI Studio](https://aistudio.google.com/apikey).
Gemini API billing is separate from other Google subscriptions. The native
`generateContent` API is used directly; no OpenAI-compatible gateway is needed.

Model candidates:

- `gemini-3.6-flash`: a fast first choice for log diagnosis.
- `gemini-3.6-pro`: a more capable choice when thoroughness matters more than
	cost and latency.

```sh
export ROTARI_LLM_PROVIDER='gemini'
export ROTARI_LLM_API_KEY='your-gemini-api-key'
export ROTARI_LLM_MODEL='gemini-3.6-flash'
```

### Cohere

Create a Cohere API key in the [Cohere dashboard](https://dashboard.cohere.com/api-keys).
Cohere API billing is separate from other Cohere subscriptions. rotari uses
the native v2 Chat API directly; no OpenAI-compatible gateway is needed.

Model candidates:

- `command-a-reasoning-08-2025`: a reasoning-oriented choice for diagnosis.
- `command-a-plus-05-2026`: a general-purpose choice when available in the
	selected Cohere account.

```sh
export ROTARI_LLM_PROVIDER='cohere'
export ROTARI_LLM_API_KEY='your-cohere-api-key'
export ROTARI_LLM_MODEL='command-a-reasoning-08-2025'
```

`ROTARI_LLM_MODEL` is not a rotari model name: pass the identifier exposed by
the configured endpoint exactly as the provider spells it. Model availability
and pricing change, so confirm the exact names in the provider's catalog. For
a local Ollama server, run `ollama list`. For OpenAI, query
`GET https://api.openai.com/v1/models` with the API key.

Do not point the `openai` provider at a Chat Completions-only endpoint. Use
`openai-chat` for endpoints that accept only `/v1/chat/completions`; the two
providers intentionally send different request and response formats.
