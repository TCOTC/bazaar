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
 * 使用 TypeScript Compiler API 检查 SiYuan 插件类是否实现了 onload 方法。
 *
 * 用法: node analyze.mjs <filename>
 *   <filename>  文件名（仅用于判断 JS/TS 脚本类型），文件内容从 stdin 读入。
 *
 * 输出（JSON，写入 stdout）：
 *   { "has_onload": boolean, "pass": boolean }
 *   或在出错时：
 *   { "error": string, "has_onload": false, "pass": false }
 */

import ts from 'typescript';

const filename = process.argv[2] || 'index.js';

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
// 主流程：从 stdin 读入文件内容，解析并检测
// ---------------------------------------------------------------------------

let source = '';
process.stdin.setEncoding('utf8');
process.stdin.on('data', (chunk) => { source += chunk; });
process.stdin.on('error', (err) => {
    process.stdout.write(JSON.stringify({ error: err.message, has_onload: false, pass: false }));
    process.exit(1);
});
process.stdin.on('end', () => {
    try {
        const scriptKind = /\.(ts|tsx|mts|cts)$/.test(filename) ? ts.ScriptKind.TS : ts.ScriptKind.JS;
        const sourceFile = ts.createSourceFile(
            filename,
            source,
            ts.ScriptTarget.Latest,
            /* setParentNodes */ true,
            scriptKind,
        );

        const hasOnload = hasOnloadMethod(sourceFile);
        process.stdout.write(JSON.stringify({ has_onload: hasOnload, pass: hasOnload }));
    } catch (err) {
        process.stdout.write(JSON.stringify({ error: err.message, has_onload: false, pass: false }));
        process.exit(1);
    }
});
