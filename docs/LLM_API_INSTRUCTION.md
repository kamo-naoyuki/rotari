# LLM diagnosis setup

This guide configures `rotari diagnose` with the OpenAI Responses API. The
same command can use an OpenAI-compatible endpoint, but obtain its API key and
model name from that provider instead.

## 1. Create an API key

Create or select an OpenAI project, then create a secret API key in the
[OpenAI API key page](https://platform.openai.com/api-keys). The project must
have API access. API billing and ChatGPT subscriptions are separate. Add API
credits in [OpenAI billing settings](https://platform.openai.com/settings/organization/billing/)
before the first request.

Copy the key when it is displayed. Do not commit it, put it in a Rotari config
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

The sample creates an isolated temporary Rotari state directory, runs a Python
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
job. Rotari does not persist the API key or the returned diagnosis.

`--language` accepts a BCP 47 language tag such as `ja`, `en`, or `en-US` and
asks the model to answer in that language. Set `ROTARI_LLM_LANGUAGE=ja` to use
the same default for multiple commands. When neither is set, the model chooses
its response language.

## Endpoint and model examples

`diagnose` requires the OpenAI **Responses API**, not only the more common
Chat Completions API. The endpoint must accept `POST /v1/responses`, a
`Bearer` authorization header, and the `model` and `input` request fields.

### OpenAI

This is the default configuration. `gpt-5-mini` is a practical first choice
for short log diagnoses; use `gpt-5` when a more thorough diagnosis is worth
the additional cost and latency.

Model candidates:

- `gpt-5-mini`: first choice for lower cost and latency.
- `gpt-5`: more thorough diagnosis when cost and latency are less important.
- `gpt-4.1-mini` and `gpt-4.1`: alternatives when they are enabled for the
	project.

```sh
export ROTARI_LLM_ENDPOINT='https://api.openai.com/v1/responses'
export ROTARI_LLM_MODEL='gpt-5-mini'
```

### Ollama on the local machine

[Ollama's OpenAI compatibility API](https://docs.ollama.com/api/openai-compatibility)
supports `POST /v1/responses` from version 0.13.3. Download a model first,
then point Rotari at the local server. The API key value is required by Rotari
but ignored by a default local Ollama server.

Model candidates:

- `qwen3:8b`: a moderate-size first choice for a local diagnosis model.
- `gpt-oss:20b`: a larger choice when the host has sufficient memory.
- `llama3.2`: use this when it is the installed local model.
- `gemma3-4b-it:q4_0`: a smaller local option; this is installed in the
	development environment used for this repository.

```sh
ollama pull qwen3:8b
export ROTARI_LLM_ENDPOINT='http://localhost:11434/v1/responses'
export ROTARI_LLM_API_KEY='ollama'
export ROTARI_LLM_MODEL='qwen3:8b'
```

If using the already installed smaller Gemma model instead, keep the endpoint
and API key above but set:

```sh
export ROTARI_LLM_MODEL='gemma3-4b-it:q4_0'
```

### Ollama cloud

Ollama also exposes a hosted Responses API. To use it for the first time:

1. Open [ollama.com](https://ollama.com/) and create an account or sign in.
2. Open [API key settings](https://ollama.com/settings/keys), create an API
	key, and copy it when shown.
3. Choose a model from the [Ollama cloud catalog](https://ollama.com/search?c=cloud).
4. Set the endpoint, copied key, and selected model name in the shell:

```sh
export ROTARI_LLM_ENDPOINT='https://ollama.com/v1/responses'
export ROTARI_LLM_API_KEY='your-ollama-api-key'
export ROTARI_LLM_MODEL='gemma4:31b'
```

`gemma4:31b` is one documented cloud model example. API keys do not expire;
revoke a key from the same API key settings page if it is exposed.

Model availability and pricing change by provider, so check its model catalog
before choosing a production default.

`ROTARI_LLM_MODEL` is not a Rotari model name: pass the identifier exposed by
the configured endpoint exactly as the provider spells it. Model availability
and pricing change, so confirm the exact names in the provider's catalog. For
a local Ollama server, run `ollama list`. For OpenAI, query
`GET https://api.openai.com/v1/models` with the API key.

### Providers that need an adapter

Do not point this version of `diagnose` at a Chat Completions-only endpoint.
For example, an endpoint that accepts only `/v1/chat/completions` cannot be
used because Rotari sends `/v1/responses` and expects a Responses API response.
Anthropic's native API also needs an adapter. A future Rotari adapter could
support those protocols directly.