# GLM Content Reviewer Plugin

GLM Content Reviewer is an Apache Answer plugin that uses ZhipuAI's content moderation API to automatically review forum content for inappropriate material.

## Features

- **Automatic content moderation** using GLM-4 API
- **Configurable review** for questions, answers, and comments independently
- **Flexible handling**: review queue or auto-delete flagged content
- **Admin bypass**: administrators are not subject to review
- **Smart caching**: reduces API calls by caching review results (1-hour TTL)
- **Rate limiting**: prevents API abuse with token bucket algorithm (10 req/sec)
- **Retry logic**: automatic retry with exponential backoff on API failures
- **Content length validation**: handles long content gracefully (8000 char limit)
- **Configurable timeout**: adjust API request timeout to your needs
- **Metrics tracking**: monitors review statistics and performance
- **Multi-language support**: English and Chinese translations

## Installation

1. Build the plugin:
```bash
cd reviewer-glm
go build
```

2. Copy the compiled plugin to your Answer plugins directory

3. Restart Answer server

## Configuration

1. Get your API key from [ZhipuAI Console](https://open.bigmodel.cn/usercenter/apikeys)

2. Configure the plugin in Answer admin panel:
   - **API Key**: Your ZhipuAI API key (required)
   - **API Timeout**: Request timeout in seconds (default: 30)
   - **Max Content Length**: Maximum content length in characters (default: 8000)
   - **Cache TTL**: Cache time-to-live in minutes (default: 60)
   - **Cache Max Size**: Maximum cached entries (default: 1000)
   - **Rate Limit RPS**: Maximum requests per second (default: 10)
   - **Max Retries**: Maximum retry attempts for failed API calls (default: 2)
   - **Review Questions**: Enable/disable question review
   - **Review Answers**: Enable/disable answer review
   - **Review Comments**: Enable/disable comment review
   - **Spam Filtering**: Choose "review queue" or "auto-delete"

## How It Works

When enabled, the plugin:
1. Intercepts new content submissions (questions/answers/comments)
2. Checks cache for previously reviewed identical content
3. Applies rate limiting to prevent API abuse
4. Sends content to GLM-4 API for moderation
5. Parses response for safety indicators
6. Approved content is published immediately
7. Flagged content is queued for review or deleted based on configuration

## Performance Features

### Caching
- Content is hashed and cached with configurable TTL (default: 1 hour)
- Configurable cache size (default: 1000 entries)
- Reduces redundant API calls for duplicate content
- Automatic cleanup of expired cache entries

### Rate Limiting
- Token bucket algorithm with configurable rate (default: 10 req/s)
- Prevents API quota exhaustion
- Graceful degradation under high load
- Adjustable to match your API plan

### Retry Logic
- Configurable retry attempts (default: 2 retries)
- Exponential backoff between retries
- Detailed error logging

### Content Validation
- Configurable content length limit (default: 8000 chars)
- Automatic truncation with warning logs
- Prevents token limit issues

### Metrics
- Tracks total reviews, approvals, rejections
- Monitors cache hit rate and API errors
- Logs statistics every 5 minutes

## API Integration

The plugin uses GLM-4 chat completion API with a specialized prompt for content moderation. Responses are parsed for safety keywords in both English and Chinese:
- Safe: "safe", "Safe", "SAFE", "安全"
- Unsafe: "unsafe", "违规", "不安全", "不合规"

## Troubleshooting

**High API errors**: Increase the API timeout value in configuration
**Rate limit exceeded**: Content is automatically rejected for review; increase rate limit RPS if needed
**Long content truncated**: Content over the configured max length is truncated before review; adjust max content length setting
**Cache not working**: Check cache TTL and max size settings; ensure values are positive
**Performance issues**: Adjust cache size and rate limit to balance performance and API costs

## License

Apache License 2.0
