// SiYuan community bazaar.
// Copyright (c) 2021-present, b3log.org
//
// Bazaar is licensed under Mulan PSL v2.
// You can use this software according to the terms and conditions of the Mulan PSL v2.
// You may obtain a copy of Mulan PSL v2 at:
//         http://license.coscl.org.cn/MulanPSL2
// THIS SOFTWARE IS PROVIDED ON AN "AS IS" BASIS, WITHOUT WARRANTIES OF ANY KIND, EITHER EXPRESS OR IMPLIED, INCLUDING BUT NOT LIMITED TO NON-INFRINGEMENT, MERCHANTABILITY OR FIT FOR A PARTICULAR PURPOSE.
// See the Mulan PSL v2 for more details.

package main

import (
"encoding/json"
"fmt"
"os"
"os/exec"
"path/filepath"
"regexp"
"strings"
)

// isValidGitRef 验证 git 引用名（tag、分支等）是否符合安全格式。
// exec.Command 不经过 shell，无注入风险，但仍对异常值做基本校验。
var gitRefPattern = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9._/\-]*$`)

func isValidGitRef(ref string) bool {
return ref != "" && len(ref) <= 255 && gitRefPattern.MatchString(ref)
}

// stripJSONComments 去除 JSON 文本中的 // 行注释和 /* */ 块注释，以便 encoding/json 可以解析 tsconfig.json 等 JSONC 文件。
func stripJSONComments(src string) string {
var b strings.Builder
b.Grow(len(src))
inString := false
i := 0
for i < len(src) {
c := src[i]
if inString {
b.WriteByte(c)
if c == '\\' && i+1 < len(src) {
i++
b.WriteByte(src[i])
} else if c == '"' {
inString = false
}
i++
continue
}
if c == '"' {
inString = true
b.WriteByte(c)
i++
continue
}
// 行注释
if c == '/' && i+1 < len(src) && src[i+1] == '/' {
for i < len(src) && src[i] != '\n' {
i++
}
continue
}
// 块注释
if c == '/' && i+1 < len(src) && src[i+1] == '*' {
i += 2
for i+1 < len(src) && !(src[i] == '*' && src[i+1] == '/') {
i++
}
if i+1 < len(src) {
i += 2 // 跳过 */
}
continue
}
b.WriteByte(c)
i++
}
return b.String()
}

// clonePluginRepo 将插件仓库在指定 tag 处浅克隆到临时目录并返回目录路径。
// 调用方负责在使用完毕后调用 os.RemoveAll(tmpDir) 清理。
func clonePluginRepo(repoOwner, repoName, tag string) (tmpDir string, err error) {
tmpDir, err = os.MkdirTemp("", "bazaar-plugin-*")
if err != nil {
return
}
repoURL := fmt.Sprintf("https://github.com/%s/%s", repoOwner, repoName)
cmd := exec.Command("git", "clone", "--depth", "1", "--branch", tag, "--no-tags", repoURL, tmpDir)
if output, cmdErr := cmd.CombinedOutput(); cmdErr != nil {
os.RemoveAll(tmpDir)
tmpDir = ""
err = fmt.Errorf("git clone [%s@%s] failed: %s: %s", repoURL, tag, cmdErr, output)
}
return
}

// resolveEntryFileLocal 从已克隆仓库的本地文件中推断源码入口文件路径。
// 解析顺序：tsconfig.json → files[0]；tsconfig.json → include 目录的候选文件；package.json → main；回退为 index.js。
func resolveEntryFileLocal(dir string) string {
// ── 1. tsconfig.json ──────────────────────────────────────────────────
if data, readErr := os.ReadFile(filepath.Join(dir, "tsconfig.json")); readErr == nil {
var tsconfig struct {
Files   []string `json:"files"`
Include []string `json:"include"`
}
if json.Unmarshal([]byte(stripJSONComments(string(data))), &tsconfig) == nil {
// files[0]：最明确的单文件入口声明
if len(tsconfig.Files) > 0 {
return tsconfig.Files[0]
}
// include：从第一个 glob 模式的根目录查找候选入口
if len(tsconfig.Include) > 0 {
includeDir := tsconfig.Include[0]
if idx := strings.IndexAny(includeDir, "*?{"); idx >= 0 {
includeDir = includeDir[:idx]
}
includeDir = strings.TrimRight(includeDir, "/\\")
includeDir = strings.TrimPrefix(includeDir, "./")
if includeDir != "" {
for _, candidate := range []string{includeDir + "/index.ts", includeDir + "/main.ts"} {
if _, statErr := os.Stat(filepath.Join(dir, candidate)); statErr == nil {
return candidate
}
}
}
}
}
}

// ── 2. package.json ───────────────────────────────────────────────────
if data, readErr := os.ReadFile(filepath.Join(dir, "package.json")); readErr == nil {
var pkg struct {
Main string `json:"main"`
}
if json.Unmarshal(data, &pkg) == nil && pkg.Main != "" {
return pkg.Main
}
}

// ── 3. 回退 ──────────────────────────────────────────────────────────
return FILE_PATH_INDEX_JS
}

// checkPluginCode 浅克隆插件仓库，通过 TypeScript Compiler API（Node.js）分析全部源码，
// 检查插件类是否实现了 onload 方法，并返回方法所在的文件、行、列信息。
func checkPluginCode(
repoOwner string,
repoName string,
tag string,
) (codeAnalysis *PluginCodeAnalysis, err error) {
codeAnalysis = &PluginCodeAnalysis{}

// 验证 tag 格式，防止传入异常值
if !isValidGitRef(tag) {
err = fmt.Errorf("invalid tag [%s] for repo [%s/%s]", tag, repoOwner, repoName)
return
}

// 浅克隆插件仓库（在 tag 处）
tmpDir, cloneErr := clonePluginRepo(repoOwner, repoName, tag)
if cloneErr != nil {
err = fmt.Errorf("clone repo [%s/%s@%s] failed: %s", repoOwner, repoName, tag, cloneErr)
return
}
defer os.RemoveAll(tmpDir)

// 从本地编译配置解析入口文件（用于无法找到 onload 时的回退显示）
entryFile := resolveEntryFileLocal(tmpDir)
codeAnalysis.EntryFile = entryFile

// 调用 Node.js 脚本分析整个项目，传入克隆目录和入口文件
scriptPath := filepath.Join("actions", "check", "plugin-analyzer", "analyze.mjs")
cmd := exec.Command("node", scriptPath, tmpDir, entryFile)
output, cmdErr := cmd.Output()
if cmdErr != nil {
if exitErr, ok := cmdErr.(*exec.ExitError); ok {
err = fmt.Errorf("plugin analyzer for repo [%s/%s] failed: %s", repoOwner, repoName, string(exitErr.Stderr))
} else {
err = fmt.Errorf("plugin analyzer for repo [%s/%s] failed: %s", repoOwner, repoName, cmdErr)
}
// 即便脚本以非零码退出，stdout 中仍可能有 JSON 结果，尝试解析
if len(output) == 0 {
return
}
}

// 解析脚本输出
var scriptOutput struct {
*PluginCodeAnalysis
OnloadFile string `json:"onload_file"` // onload 方法所在文件（相对路径）
Error      string `json:"error"`
}
scriptOutput.PluginCodeAnalysis = codeAnalysis
if jsonErr := json.Unmarshal(output, &scriptOutput); jsonErr != nil {
if err == nil {
err = fmt.Errorf("parse plugin analyzer output for repo [%s/%s] failed: %s", repoOwner, repoName, jsonErr)
}
return
}

// 用脚本返回的实际文件路径覆盖入口文件（若 onload 存在于其他文件中）
if scriptOutput.OnloadFile != "" {
codeAnalysis.EntryFile = scriptOutput.OnloadFile
}

if scriptOutput.Error != "" {
if err == nil {
err = fmt.Errorf("plugin analyzer for repo [%s/%s] reported error: %s", repoOwner, repoName, scriptOutput.Error)
}
}

return
}
