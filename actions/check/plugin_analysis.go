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
	"fmt"
	"net/http"
	"regexp"

	"github.com/parnurzeal/gorequest"
	"github.com/siyuan-note/bazaar/actions/util"
)

// onloadPattern matches common JavaScript/TypeScript forms of the onload method:
//   - standard method: onload() or async onload()
//   - property assignment: onload = () => or onload = async () =>
// Note: this is a simple text-based check; it may match in comments or strings,
// but for bundled plugin code this provides a practical and fast analysis.
var onloadPattern = regexp.MustCompile(`\bonload\s*\(|\bonload\s*=\s*(?:async\s*)?\(`)

// checkPluginCode 对插件 index.js 进行静态分析，检查插件类是否实现了 onload 方法
func checkPluginCode(
	repoOwner string,
	repoName string,
	hash string,
) (codeAnalysis *PluginCodeAnalysis, err error) {
	codeAnalysis = &PluginCodeAnalysis{}

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

	// 检查 index.js 中是否有 onload 方法
	codeAnalysis.HasOnload = onloadPattern.Match(data)
	codeAnalysis.Pass = codeAnalysis.HasOnload

	return
}
