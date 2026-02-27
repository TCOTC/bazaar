// SiYuan community bazaar.
// Copyright (c) 2021-present, b3log.org
//
// Bazaar is licensed under Mulan PSL v2.
// You can use this software according to the terms and conditions of the Mulan PSL v2.
// You may obtain a copy of Mulan PSL v2 at:
//         http://license.coscl.org.cn/MulanPSL2
// THIS SOFTWARE IS PROVIDED ON AN "AS IS" BASIS, WITHOUT WARRANTIES OF ANY KIND, EITHER EXPRESS OR IMPLIED, INCLUDING BUT NOT LIMITED TO NON-INFRINGEMENT, MERCHANTABILITY OR FIT FOR A PARTICULAR PURPOSE.
// See the Mulan PSL v2 for more details.

/**
 * 使用 TypeScript Compiler API 分析 SiYuan 插件源码，检查插件类是否实现了 onload 方法。
 *
 * 用法: node analyze.mjs <owner> <repo> <hash>
 *
 * 入口文件解析顺序：
 *   1. tsconfig.json → files[0]（显式文件列表）
 *   2. tsconfig.json → include 目录下的 index.ts / main.ts
 *   3. package.json  → main 字段
 *   4. 回退为 index.js
 *
 * 输出（JSON，写入 stdout）：
 *   { "entryFile": string, "hasOnload": boolean, "pass": boolean }
 *   或在出错时：
 *   { "error": string, "entryFile": "", "hasOnload": false, "pass": false }
 */

import ts from 'typescript';
import https from 'https';

const [,, owner, repo, hash] = process.argv;

// ---------------------------------------------------------------------------
// HTTP helper
// ---------------------------------------------------------------------------

/**
 * 发起 GET 请求并返回响应文本；若状态码非 200 则抛出错误。
 * @param {string} url
 * @returns {Promise<string>}
 */
function fetchText(url) {
    return new Promise((resolve, reject) => {
        const options = { headers: { 'User-Agent': 'bazaar-plugin-analyzer' } };
        https.get(url, options, (res) => {
            // 跟随重定向（raw.githubusercontent.com 偶尔会跳转）
            if (res.statusCode === 301 || res.statusCode === 302) {
                fetchText(res.headers.location).then(resolve).catch(reject);
                return;
            }
            if (res.statusCode !== 200) {
                reject(new Error(`HTTP ${res.statusCode} for ${url}`));
                return;
            }
            let data = '';
            res.on('data', (chunk) => { data += chunk; });
            res.on('end', () => resolve(data));
        }).on('error', reject);
    });
}

/**
 * 尝试获取文件内容；若请求失败则返回 null。
 * @param {string} path  仓库内相对路径
 * @returns {Promise<string|null>}
 */
async function tryFetch(path) {
    try {
        return await fetchText(`https://raw.githubusercontent.com/${owner}/${repo}/${hash}/${path}`);
    } catch {
        return null;
    }
}

// ---------------------------------------------------------------------------
// 入口文件解析
// ---------------------------------------------------------------------------

/**
 * 从 tsconfig.json、package.json 等编译配置中推断插件入口文件路径。
 * @returns {Promise<string>}
 */
async function resolveEntryFile() {
    // ── 1. tsconfig.json ──────────────────────────────────────────────────
    const tsconfigText = await tryFetch('tsconfig.json');
    if (tsconfigText) {
        try {
            // 使用 TypeScript 内置解析器处理含注释的 JSON
            const { config, error } = ts.parseConfigFileTextToJson('tsconfig.json', tsconfigText);
            if (!error && config) {
                // files[0]：最明确的单文件入口声明
                if (Array.isArray(config.files) && config.files.length > 0) {
                    return config.files[0];
                }
                // include：从第一个 glob 模式的根目录查找候选入口
                if (Array.isArray(config.include) && config.include.length > 0) {
                    const dir = config.include[0].replace(/[/\\]\*.*$/, '').replace(/[/\\]$/, '');
                    if (dir) {
                        const candidates = [
                            `${dir}/index.ts`,
                            `${dir}/main.ts`,
                            `${dir}/index.js`,
                        ];
                        for (const candidate of candidates) {
                            if (await tryFetch(candidate) !== null) {
                                return candidate;
                            }
                        }
                    }
                }
            }
        } catch {
            // tsconfig.json 解析失败，继续尝试下一个配置源
        }
    }

    // ── 2. package.json ───────────────────────────────────────────────────
    const packageText = await tryFetch('package.json');
    if (packageText) {
        try {
            const pkg = JSON.parse(packageText);
            if (typeof pkg.main === 'string' && pkg.main) {
                return pkg.main;
            }
        } catch {
            // package.json 解析失败，继续回退
        }
    }

    // ── 3. 回退 ──────────────────────────────────────────────────────────
    return 'index.js';
}

// ---------------------------------------------------------------------------
// onload 检测
// ---------------------------------------------------------------------------

/**
 * 递归遍历 TypeScript AST，查找名为 onload 的方法声明（支持 JS/TS）。
 * @param {ts.Node} node
 * @returns {boolean}
 */
function hasOnloadMethod(node) {
    if (ts.isMethodDeclaration(node)) {
        const name = node.name;
        if (ts.isIdentifier(name) && name.text === 'onload') {
            return true;
        }
    }
    return ts.forEachChild(node, hasOnloadMethod) === true;
}

// ---------------------------------------------------------------------------
// 主流程
// ---------------------------------------------------------------------------

async function main() {
    let entryFile = '';
    try {
        entryFile = await resolveEntryFile();
        const source = await fetchText(
            `https://raw.githubusercontent.com/${owner}/${repo}/${hash}/${entryFile}`,
        );

        const scriptKind = /\.(ts|tsx|mts|cts)$/.test(entryFile) ? ts.ScriptKind.TS : ts.ScriptKind.JS;
        const sourceFile = ts.createSourceFile(
            entryFile,
            source,
            ts.ScriptTarget.Latest,
            /* setParentNodes */ true,
            scriptKind,
        );

        const hasOnload = hasOnloadMethod(sourceFile);
        process.stdout.write(JSON.stringify({ entryFile, hasOnload, pass: hasOnload }));
    } catch (err) {
        process.stdout.write(JSON.stringify({
            error: err.message,
            entryFile,
            hasOnload: false,
            pass: false,
        }));
        process.exit(1);
    }
}

main();
