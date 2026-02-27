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
	"os/exec"
	"path/filepath"
)

// checkPluginCode 调用 Node.js 分析脚本，通过 TypeScript Compiler API 检查插件类是否实现了 onload 方法。
// 入口文件路径从插件仓库的编译配置 (tsconfig.json / package.json) 中解析，无法确定时回退为 index.js。
func checkPluginCode(
	repoOwner string,
	repoName string,
	hash string,
) (codeAnalysis *PluginCodeAnalysis, err error) {
	codeAnalysis = &PluginCodeAnalysis{}

	scriptPath := filepath.Join("actions", "check", "plugin-analyzer", "analyze.mjs")
	cmd := exec.Command("node", scriptPath, repoOwner, repoName, hash)
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

	var result struct {
		EntryFile string `json:"entryFile"`
		HasOnload bool   `json:"hasOnload"`
		Pass      bool   `json:"pass"`
		Error     string `json:"error"`
	}
	if jsonErr := json.Unmarshal(output, &result); jsonErr != nil {
		if err == nil {
			err = fmt.Errorf("parse plugin analyzer output for repo [%s/%s] failed: %s", repoOwner, repoName, jsonErr)
		}
		return
	}

	if result.Error != "" {
		if err == nil {
			err = fmt.Errorf("plugin analyzer for repo [%s/%s] reported error: %s", repoOwner, repoName, result.Error)
		}
	}

	codeAnalysis.EntryFile = result.EntryFile
	codeAnalysis.HasOnload = result.HasOnload
	codeAnalysis.Pass = result.Pass

	return
}

