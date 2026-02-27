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
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/parnurzeal/gorequest"
	"github.com/siyuan-note/bazaar/actions/util"
)

// fetchRawFile 下载仓库中指定路径的原始文件内容；若请求失败或状态码非 200 则返回 error。
func fetchRawFile(repoOwner, repoName, hash, filePath string) ([]byte, error) {
	rawUrl := buildFileRawURL(repoOwner, repoName, hash, filePath)
	response, data, errs := gorequest.
		New().
		Get(rawUrl).
		Set("User-Agent", util.UserAgent).
		Retry(REQUEST_RETRY_COUNT, REQUEST_RETRY_DURATION).
		Timeout(REQUEST_TIMEOUT).
		EndBytes()
	if len(errs) > 0 {
		return nil, fmt.Errorf("HTTP GET [%s] failed: %v", rawUrl, errs)
	}
	if response.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HTTP GET [%s] returned %s", rawUrl, response.Status)
	}
	return data, nil
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

// resolveEntryFile 从插件仓库的编译配置中推断源码入口文件路径。
// 解析顺序：tsconfig.json → files[0]；tsconfig.json → include 目录的候选文件；package.json → main；回退为 index.js。
func resolveEntryFile(repoOwner, repoName, hash string) string {
	// ── 1. tsconfig.json ──────────────────────────────────────────────────
	if tsconfigData, err := fetchRawFile(repoOwner, repoName, hash, "tsconfig.json"); err == nil {
		var tsconfig struct {
			Files   []string `json:"files"`
			Include []string `json:"include"`
		}
		if json.Unmarshal([]byte(stripJSONComments(string(tsconfigData))), &tsconfig) == nil {
			// files[0]：最明确的单文件入口声明
			if len(tsconfig.Files) > 0 {
				return tsconfig.Files[0]
			}
			// include：从第一个 glob 模式的根目录查找候选入口
			if len(tsconfig.Include) > 0 {
				dir := tsconfig.Include[0]
				// 去掉 glob 后缀（如 /**/*、/*.ts）
				if idx := strings.IndexAny(dir, "*?{"); idx >= 0 {
					dir = dir[:idx]
				}
				dir = strings.TrimRight(dir, "/\\")
				dir = strings.TrimPrefix(dir, "./")
				if dir != "" {
					for _, candidate := range []string{dir + "/index.ts", dir + "/main.ts"} {
						if _, err := fetchRawFile(repoOwner, repoName, hash, candidate); err == nil {
							return candidate
						}
					}
				}
			}
		}
	}

	// ── 2. package.json ───────────────────────────────────────────────────
	if pkgData, err := fetchRawFile(repoOwner, repoName, hash, "package.json"); err == nil {
		var pkg struct {
			Main string `json:"main"`
		}
		if json.Unmarshal(pkgData, &pkg) == nil && pkg.Main != "" {
			return pkg.Main
		}
	}

	// ── 3. 回退 ──────────────────────────────────────────────────────────
	return FILE_PATH_INDEX_JS
}

// checkPluginCode 通过 TypeScript Compiler API（Node.js）检查插件类是否实现了 onload 方法。
// 优先分析源码入口文件（从 tsconfig.json / package.json 解析），以便获取准确的文件行列信息；
// 若无法确定源码入口，则回退为分析编译产物 index.js。
func checkPluginCode(
	repoOwner string,
	repoName string,
	hash string,
) (codeAnalysis *PluginCodeAnalysis, err error) {
	codeAnalysis = &PluginCodeAnalysis{}

	// 解析源码入口文件路径
	entryFile := resolveEntryFile(repoOwner, repoName, hash)
	codeAnalysis.EntryFile = entryFile

	// 下载入口文件内容
	data, fetchErr := fetchRawFile(repoOwner, repoName, hash, entryFile)
	if fetchErr != nil {
		err = fmt.Errorf("download entry file [%s] for repo [%s/%s] failed: %s", entryFile, repoOwner, repoName, fetchErr)
		return
	}

	// 调用 Node.js 脚本解析，将文件内容通过 stdin 传入
	scriptPath := filepath.Join("actions", "check", "plugin-analyzer", "analyze.mjs")
	cmd := exec.Command("node", scriptPath, entryFile)
	cmd.Stdin = bytes.NewReader(data)
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

	// 解析脚本输出；脚本输出的 JSON 字段名与 PluginCodeAnalysis 的 tag 一致，直接反序列化
	var scriptOutput struct {
		*PluginCodeAnalysis
		Error string `json:"error"`
	}
	scriptOutput.PluginCodeAnalysis = codeAnalysis
	if jsonErr := json.Unmarshal(output, &scriptOutput); jsonErr != nil {
		if err == nil {
			err = fmt.Errorf("parse plugin analyzer output for repo [%s/%s] failed: %s", repoOwner, repoName, jsonErr)
		}
		return
	}

	if scriptOutput.Error != "" {
		if err == nil {
			err = fmt.Errorf("plugin analyzer for repo [%s/%s] reported error: %s", repoOwner, repoName, scriptOutput.Error)
		}
	}

	return
}

