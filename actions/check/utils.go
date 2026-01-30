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
	"os"
	"regexp"
	"strings"

	"github.com/88250/gulu"
)

var (
	// 文件/目录名称保留字
	RESERVED_WORDS = StringSet{
		"CON":  nil,
		"PRN":  nil,
		"AUX":  nil,
		"NUL":  nil,
		"COM0": nil,
		"COM1": nil,
		"COM2": nil,
		"COM3": nil,
		"COM4": nil,
		"COM5": nil,
		"COM6": nil,
		"COM7": nil,
		"COM8": nil,
		"COM9": nil,
		"LPT0": nil,
		"LPT1": nil,
		"LPT2": nil,
		"LPT3": nil,
		"LPT4": nil,
		"LPT5": nil,
		"LPT6": nil,
		"LPT7": nil,
		"LPT8": nil,
		"LPT9": nil,
	}
)

// isKeyInSet 判断字符串是否在集合中
func isKeyInSet(
	key string,
	set StringSet,
) (exist bool) {
	_, exist = set[key]

	return
}

// isValidName 判断资源名称是否有效
func isValidName(name string) (valid bool) {
	var err error

	// 是否为空字符串
	if name == "" {
		logger.Warnf("name is empty")
		return
	}

	// 是否均为可打印的 ASCii 字符
	if valid, err = regexp.MatchString("^[\\x20-\\x7E]+$", name); err != nil {
		panic(err)
	} else if !valid {
		logger.Warnf("name <\033[7m%s\033[0m> contains characters other than printable ASCII characters", name)
		return
	}

	// 是否均为有效字符
	if valid, err = regexp.MatchString("^[^\\\\/:*?\"<>|. ][^\\\\/:*?\"<>|]*[^\\\\/:*?\"<>|. ]$", name); err != nil {
		panic(err)
	} else if !valid {
		logger.Warnf("name <\033[7m%s\033[0m> contains invalid characters", name)
		return
	}

	// 是否为保留字
	// REF https://learn.microsoft.com/zh-cn/windows/win32/fileio/naming-a-file#naming-conventions
	if valid = !isKeyInSet(strings.ToUpper(name), RESERVED_WORDS); !valid {
		logger.Warnf("name <\033[7m%s\033[0m> is a reserved word", name)
		return
	}

	return
}

// buildFileRawURL 构造文件原始访问地址
func buildFileRawURL(
	repoOwner string,
	repoName string,
	hash string,
	filePath string,
) string {
	return fmt.Sprintf("https://raw.githubusercontent.com/%s/%s/%s/%s", repoOwner, repoName, hash, filePath)
}

// buildFilePreviewURL 构造文件预览地址
func buildFilePreviewURL(
	repoOwner string,
	repoName string,
	hash string,
	filePath string,
) string {
	return fmt.Sprintf("https://github.com/%s/%s/blob/%s/%s", repoOwner, repoName, hash, filePath)
}

// buildRepoHomeURL 构造仓库主页地址
func buildRepoHomeURL(
	repoOwner string,
	repoName string,
) string {
	return fmt.Sprintf("https://github.com/%s/%s", repoOwner, repoName)
}

// getNewReposFromGitDiff 获取 PR 中真正新增的仓库
// 使用 JSON 解析方法：比较 PR head 和 base 的 JSON 文件，然后过滤掉已在 main 中存在的仓库
// 这种方法简单可靠，不依赖 git 历史，且最终结果与 git diff 方法一致（因为都需要过滤 main 分支）
func getNewReposFromGitDiff(prRepoPath string, baseRepoPath string, targetFilePath string) []string {
	// 获取文件名
	fileName := getFileNameFromPath(targetFilePath)

	// 使用 JSON 解析方法：比较 PR head 和 base 的文件
	baseFilePath := baseRepoPath + "/" + fileName
	candidates := getNewReposFromFileComparison(baseFilePath, targetFilePath)

	// 如果候选列表为空，直接返回
	if len(candidates) == 0 {
		return candidates
	}

	// 读取 main 分支的最新状态，过滤掉那些已经在 main 中存在的仓库
	// 这样可以避免将解决冲突时合并过来的仓库误判为新增
	mainRepoPath := MAIN_REPO_PATH
	if mainRepoPath == "" {
		// 如果未设置 MAIN_REPO_PATH，使用 baseRepoPath（向后兼容）
		mainRepoPath = baseRepoPath
	}
	mainFilePath := mainRepoPath + "/" + fileName
	mainFile, err := os.ReadFile(mainFilePath)
	if err != nil {
		// 如果无法读取 main 分支的文件，返回所有候选仓库（保守策略）
		logger.Warnf("failed to read main branch file, returning all candidates: %s", err)
		return candidates
	}

	main := map[string]interface{}{}
	if err = gulu.JSON.UnmarshalJSON(mainFile, &main); err != nil {
		logger.Warnf("failed to unmarshal main branch file, returning all candidates: %s", err)
		return candidates
	}

	mainRepos := main["repos"].([]interface{})
	mainRepoSet := make(StringSet, len(mainRepos))
	for _, mainRepo := range mainRepos {
		mainRepoPath := mainRepo.(string)
		mainRepoSet[mainRepoPath] = nil
	}

	// 过滤：只保留那些不在 main 分支中的仓库
	newRepos := []string{}
	for _, candidate := range candidates {
		if !isKeyInSet(candidate, mainRepoSet) {
			newRepos = append(newRepos, candidate)
		}
	}

	return newRepos
}

// getFileNameFromPath 从文件路径中提取文件名
func getFileNameFromPath(filePath string) string {
	parts := strings.Split(filePath, "/")
	return parts[len(parts)-1]
}

// getNewReposFromFileComparison 通过文件比较获取新增的仓库（回退方案）
func getNewReposFromFileComparison(baseFilePath string, targetFilePath string) []string {
	newRepos := []string{}

	// 读取 base 分支中的文件
	baseFile, err := os.ReadFile(baseFilePath)
	if nil != err {
		logger.Warnf("read base file <\033[7m%s\033[0m> failed: %s", baseFilePath, err)
		return newRepos
	}
	base := map[string]interface{}{}
	if err = gulu.JSON.UnmarshalJSON(baseFile, &base); nil != err {
		logger.Warnf("unmarshal base file <\033[7m%s\033[0m> failed: %s", baseFilePath, err)
		return newRepos
	}

	// 读取 PR 中的文件
	targetFile, err := os.ReadFile(targetFilePath)
	if nil != err {
		logger.Warnf("read target file <\033[7m%s\033[0m> failed: %s", targetFilePath, err)
		return newRepos
	}
	target := map[string]interface{}{}
	if err = gulu.JSON.UnmarshalJSON(targetFile, &target); nil != err {
		logger.Warnf("unmarshal target file <\033[7m%s\033[0m> failed: %s", targetFilePath, err)
		return newRepos
	}

	// 获取新增的仓库列表
	targetRepos := target["repos"].([]interface{}) // PR 中的仓库列表
	baseRepos := base["repos"].([]interface{})     // base 分支中的仓库列表
	baseRepoSet := make(StringSet, len(baseRepos)) // base 分支中的仓库 owner/name 集合
	for _, baseRepo := range baseRepos {
		baseUrl := baseRepo.(string)
		baseRepoSet[baseUrl] = nil
	}

	for _, targetRepo := range targetRepos {
		targetRepoPath := targetRepo.(string)
		if !isKeyInSet(targetRepoPath, baseRepoSet) {
			newRepos = append(newRepos, targetRepoPath)
		}
	}

	return newRepos
}
