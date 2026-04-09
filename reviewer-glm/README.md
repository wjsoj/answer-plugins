# GLM Content Reviewer

[![Apache Answer Plugin](https://img.shields.io/badge/Apache%20Answer-Plugin-blue)](https://answer.apache.org)
[![Go](https://img.shields.io/badge/Go-1.23+-00ADD8?logo=go&logoColor=white)](https://golang.org/)
[![Version](https://img.shields.io/badge/version-1.0.0-green)](info.yaml)

An [Apache Answer](https://answer.apache.org) reviewer plugin that uses [ZhipuAI](https://open.bigmodel.cn)'s GLM-4 model to automatically moderate forum content. It intercepts questions, answers, and comments before publication and flags or removes inappropriate material — spam, explicit content, violence, and more.

## Features

- **AI-powered moderation** via GLM-4 chat completion API
- **Per-content-type control** — enable/disable review for questions, answers, and comments independently
- **Flexible actions** — send flagged content to review queue or auto-delete
- **Admin bypass** — administrators skip moderation entirely
- **In-memory caching** — SHA256-based content cache with configurable TTL to reduce API calls
- **Rate limiting** — token bucket algorithm prevents API quota exhaustion
- **Retry with backoff** — automatic exponential backoff on transient API failures
- **Observability** — metrics logged every 5 minutes (approvals, rejections, cache hits, errors)
- **48+ languages** — full i18n support for the admin configuration UI

## Prerequisites

- [Go](https://golang.org/) >= 1.23
- [Apache Answer](https://github.com/apache/answer) >= 1.0
- A ZhipuAI API key ([get one here](https://open.bigmodel.cn/usercenter/apikeys))

## Installation

```bash
git clone https://github.com/wjsoj/answer-plugins.git
cd answer-plugins/reviewer-glm
go build
```

Copy the compiled plugin to your Answer plugins directory and restart the server.

> [!TIP]
> Refer to the [Apache Answer plugin documentation](https://answer.apache.org/docs/plugins) for detailed instructions on installing third-party plugins.

## Configuration

After installation, configure the plugin from the **Admin Panel > Plugins** page.

| Parameter | Default | Description |
|---|---|---|
| **API Key** | *(required)* | Your ZhipuAI API key |
| **API Timeout** | `30` s | HTTP request timeout |
| **Max Content Length** | `8000` chars | Content is truncated beyond this limit |
| **Cache TTL** | `60` min | How long review results are cached |
| **Cache Max Size** | `1000` entries | Maximum number of cached results |
| **Rate Limit RPS** | `10` req/s | Maximum API requests per second |
| **Max Retries** | `2` | Retry attempts on API failure |
| **Review Questions** | `true` | Moderate new questions |
| **Review Answers** | `true` | Moderate new answers |
| **Review Comments** | `true` | Moderate new comments |
| **Spam Filtering** | `review` | Action for flagged content: `review` (queue) or `delete` |

## How it works

```
User submits content
        |
        v
  Admin bypass? ──yes──> Publish immediately
        |no
        v
  Cache hit? ──yes──> Return cached result
        |no
        v
  Rate limit OK? ──no──> Queue for manual review
        |yes
        v
  Call GLM-4 API (with retry)
        |
    ┌───┴───┐
   Safe   Unsafe
    |        |
 Publish   Review queue / Auto-delete
```

The plugin sends content to GLM-4 with a moderation-focused system prompt and parses the response for safety keywords in both English and Chinese (`safe` / `安全` vs `unsafe` / `违规`).

> [!NOTE]
> Content that triggers ZhipuAI's built-in content filter (error code `1301`) is automatically flagged for review without retrying.

## Running tests

```bash
cd reviewer-glm
go test -v ./...
```

The test suite covers rate limiting, caching, metrics, configuration parsing, and edge cases.

## Project structure

```
reviewer-glm/
├── basic.go          # Plugin implementation
├── basic_test.go     # Test suite
├── info.yaml         # Plugin metadata
├── go.mod            # Go module definition
└── i18n/
    ├── translation.go  # i18n key constants
    └── *.yaml          # Translation files (48+ languages)
```
