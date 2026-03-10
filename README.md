# Apache Answer Plugins

<a href="https://answer.apache.org">
  <img alt="Apache Answer" src="https://answer.apache.org/img/logo.svg" height="80px">
</a>

[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](LICENSE)
[![Language](https://img.shields.io/badge/language-go-blue.svg)](https://golang.org/)

## About Apache Answer

[Apache Answer](https://answer.apache.org) is a Q&A platform software for teams at any scales. Whether it's a community forum, help center, or knowledge management platform, you can always count on Answer.

Apache Answer is an open-source project licensed under Apache License 2.0, and is a top-level project at the Apache Software Foundation.

### Key Features

- **Q&A Platform**: Full-featured question and answer system
- **Plugin System**: Extensible architecture for custom plugins
- **Multi-language Support**: Internationalization built-in
- **User Management**: Role-based access control
- **Content Moderation**: Built-in moderation tools

## Available Plugins

This repository contains plugins for Apache Answer that extend its functionality.

### reviewer-glm

GLM Content Reviewer is an Apache Answer plugin that uses ZhipuAI's GLM-4 API to automatically review forum content for inappropriate material.

**Features:**
- Automatic content moderation using GLM-4 API
- Configurable review for questions, answers, and comments
- Smart caching with configurable TTL
- Rate limiting to prevent API abuse
- Multi-language support (English and Chinese)

For more details, see [reviewer-glm/README.md](reviewer-glm/README.md).

## Installation

### Build from Source

```bash
# Clone the repository
git clone https://github.com/wjsoj/answer-plugins.git
cd answer-plugins

# Build a specific plugin
cd reviewer-glm
go build
```

### Using Plugins

1. Build or download the plugin binary
2. Copy to your Apache Answer plugins directory
3. Restart the Answer server
4. Configure via the admin panel

## Requirements

- Go >= 1.23
- Apache Answer >= 1.0

## License

This project is licensed under the MIT License - see the [LICENSE](LICENSE) file for details.

## Contributing

Contributions are welcome! Please feel free to submit a Pull Request.

## Links

- [Apache Answer Official Site](https://answer.apache.org)
- [Apache Answer GitHub](https://github.com/apache/answer)
- [Plugin Documentation](https://answer.apache.org/docs/plugins)
- [ZhipuAI Console](https://open.bigmodel.cn)
