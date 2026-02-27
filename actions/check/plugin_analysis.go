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

	"github.com/parnurzeal/gorequest"
	"github.com/siyuan-note/bazaar/actions/util"
)

// checkPluginCode 下载发行版 index.js，通过 TypeScript Compiler API（Node.js）检查插件类是否实现了 onload 方法。
// SiYuan 要求插件发行包中必须包含编译打包后的 index.js，该文件是自包含的，可直接进行静态分析。
func checkPluginCode(
	repoOwner string,
	repoName string,
	hash string,
) (codeAnalysis *PluginCodeAnalysis, err error) {
	codeAnalysis = &PluginCodeAnalysis{}

	// 下载发行版 index.js（SiYuan 要求所有插件发行包必须包含该文件）
	rawUrl := buildFileRawURL(repoOwner, repoName, hash, FILE_PATH_INDEX_JS)
	response, data, errs := gorequest.
		New().
		Get(rawUrl).
		Set("User-Agent", util.UserAgent).
		Retry(REQUEST_RETRY_COUNT, REQUEST_RETRY_DURATION).
		Timeout(REQUEST_TIMEOUT).
		EndBytes()
	if nil != errs {
		err = fmt.Errorf("HTTP GET request [%s] failed: %s", rawUrl, errs)
		return
	}
	if response.StatusCode != http.StatusOK {
		err = fmt.Errorf("HTTP GET request [%s] failed: %s", rawUrl, response.Status)
		return
	}

	// 调用 Node.js 脚本解析 index.js，将文件内容通过 stdin 传入
	scriptPath := filepath.Join("actions", "check", "plugin-analyzer", "analyze.mjs")
	cmd := exec.Command("node", scriptPath, FILE_PATH_INDEX_JS)
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

