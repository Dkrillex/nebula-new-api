## 一、安装 Node.js（已安装可跳过）
确保 Node.js 版本 ≥ 18.0，下载链接如下：
```js
# windows 用户
https://nodejs.org/dist/v22.18.0/node-v22.18.0-x64.msi

# macOS 用户
sudo xcode-select --install
/bin/bash -c "$(curl -fsSL https://raw.githubusercontent.com/Homebrew/install/HEAD/install.sh)"
brew install node
node --version

# Ubuntu / Debian 用户
curl -fsSL https://deb.nodesource.com/setup_lts.x | sudo bash -
sudo apt-get install -y nodejs
node --version


```

### 如果安装其他的 中转客户端cli 记得先卸载 （可选）
#### 第一步：检查安装位置
检查是否在本地项目中安装
```js
npm ls @anthropic-ai/claude-code
```
检查是否被全局安装
```js
npm ls -g @anthropic-ai/claude-code
```
#### 第二步：执行卸载操作
卸载本地安装的包
```js
npm uninstall @anthropic-ai/claude-code
```
卸载全局安装的包
```js
npm uninstall -g @anthropic-ai/claude-code
```
## 二、安装 Claude Code
#### 安装claude code
```js
npm install -g @anthropic-ai/claude-code
```
#### 检查是否安装成功
```js
claude --version
```


## 三、开始使用
获取URL地址和API密钥
```js
base_url = "https://llm.ai-nebula.com"
api_key = "sk-zbTYx*******************************fceTvn5"
```


## 四、配置环境变量
- Mac和Linux 环境变量
```js
export ANTHROPIC_BASE_URL="https://llm.ai-nebula.com"
export ANTHROPIC_AUTH_TOKEN="sk-zbTYx*******************************fceTvn5"
export ANTHROPIC_MODEL="kimi-k2-250905"
cd your-project-folder
claude
```

- Windows cmd命令行环境变量

```js
set ANTHROPIC_BASE_URL=https://llm.ai-nebula.com
set ANTHROPIC_AUTH_TOKEN=sk-zbTYx*******************************fceTvn5
set ANTHROPIC_MODEL=kimi-k2-250905
cd your-project-folder
claude
```

- Windows PowerShell环境变量

```js
$env:ANTHROPIC_BASE_URL="https://llm.ai-nebula.com"; 
$env:ANTHROPIC_AUTH_TOKEN="sk-zbTYx*******************************fceTvn5"
$env:ANTHROPIC_MODEL="kimi-k2-250905"
cd your-project-folder
claude
```
即可使用 Claude Code

