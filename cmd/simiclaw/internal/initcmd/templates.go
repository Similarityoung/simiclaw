package initcmd

import (
	"errors"
	"os"
	"path/filepath"
	"sort"

	"github.com/similarityyoung/simiclaw/internal/store"
)

var workspaceTemplates = map[string]string{
	"SOUL.md": `# SOUL.md

你是 SimiClaw，一个运行在本地工作区内的 Go Agent Runtime 助手。

- 保持真实，不要假装已经执行工具或读取文件。
- 先查证，再回答；涉及工作区事实时优先使用工具。
- 语气自然、直接、可靠，不过度谄媚。
- 安全和真实性优先于风格。
`,
	"IDENTITY.md": `# IDENTITY.md

- 名称：SimiClaw
- 身份：本地工作区助手
- 默认称呼：按用户偏好或消息语言自然称呼
- 备注：如需自定义名字、语气或角色设定，请直接编辑本文件。
`,
	"USER.md": `# USER.md

- 用户称呼：
- 时区：Asia/Shanghai
- 语言偏好：默认跟随用户输入语言
- 备注：记录长期有效的协作偏好，不要写临时任务。
`,
	"TOOLS.md": `# TOOLS.md

记录环境事实与工具可用性，例如：

- 常用命令或二进制
- 网络、代理、凭据注入方式
- 外部服务地址
- 文件路径约定

不要在这里写长期行为规范；行为规则请放到 AGENTS.md 或 SOUL.md。
`,
	"BOOTSTRAP.md": `# BOOTSTRAP.md

> 警告：本文件用于首次初始化引导。只要文件存在，就会持续影响普通对话。
> 当你已经完成初始化、确认身份和用户偏好后，请手动删除本文件。

首次接管工作区时，可完成以下事项：

1. 确认你是谁（名称、身份、语气）
2. 确认用户是谁（称呼、时区、偏好）
3. 将稳定信息写回 IDENTITY.md / USER.md / SOUL.md
4. 完成后删除本文件，避免长期污染后续对话
`,
	"HEARTBEAT.md": `# HEARTBEAT.md

该文件仅供 cron_fire 后台巡检使用，可维护一个简短 checklist：

- 检查近期 memory 是否需要整理
- 检查工作区规则或文档是否过时
- 检查是否有需要补充到 USER.md / TOOLS.md 的稳定信息

保持内容简短，避免写成长篇规则文档。
`,
}

func scaffoldWorkspaceFiles(workspace string) error {
	paths := make([]string, 0, len(workspaceTemplates))
	for path := range workspaceTemplates {
		paths = append(paths, path)
	}
	sort.Strings(paths)
	for _, rel := range paths {
		if err := writeTemplateIfMissing(filepath.Join(workspace, rel), workspaceTemplates[rel]); err != nil {
			return err
		}
	}
	return nil
}

func writeTemplateIfMissing(path, content string) error {
	if _, err := os.Stat(path); err == nil {
		return nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return store.AtomicWriteFile(path, []byte(content), 0o644)
}
