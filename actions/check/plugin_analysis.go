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

	"github.com/parnurzeal/gorequest"
	"github.com/siyuan-note/bazaar/actions/util"
	"github.com/tdewolff/parse/v2"
	"github.com/tdewolff/parse/v2/js"
)

// onloadVisitor 通过 AST 遍历检查是否存在 onload 方法声明
type onloadVisitor struct {
	found bool
}

func (v *onloadVisitor) Enter(n js.INode) js.IVisitor {
	if v.found {
		return nil // 已找到，停止遍历
	}
	if method, ok := n.(*js.MethodDecl); ok {
		if method.Name.IsIdent([]byte("onload")) {
			v.found = true
			return nil
		}
	}
	return v
}

func (v *onloadVisitor) Exit(n js.INode) {}

// checkPluginCode 下载插件 index.js 并通过 JS AST 解析检查插件类是否实现了 onload 方法
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

	// 解析 JS AST
	ast, parseErr := js.Parse(parse.NewInputBytes(data), js.Options{})
	if parseErr != nil {
		err = fmt.Errorf("parse index.js for repo [%s/%s] failed: %s", repoOwner, repoName, parseErr)
		return
	}

	// 遍历 AST，查找 onload 方法声明
	v := &onloadVisitor{}
	js.Walk(v, ast)

	codeAnalysis.HasOnload = v.found
	codeAnalysis.Pass = codeAnalysis.HasOnload

	return
}
