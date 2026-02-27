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
 * 使用 TypeScript Compiler API 分析整个插件项目，检查插件类是否实现了 onload 方法。
 * 通过 ts.createProgram() 解析完整的 TypeScript 项目（包括跨文件的 import），
 * 可以准确定位 onload 方法所在的源文件、行、列。
 *
 * 用法: node analyze.mjs <dir> <entry_file>
 *   <dir>         浅克隆后的仓库根目录（绝对路径）
 *   <entry_file>  入口文件相对路径（从 tsconfig/package.json 解析得到），用作无 tsconfig 时的回退
 *
 * 输出（JSON，写入 stdout）：
 *   { "has_onload": true, "pass": true, "onload_file": "src/index.ts", "onload_line": 10, "onload_column": 3 }
 *   { "has_onload": false, "pass": false }
 *   或在出错时：
 *   { "error": string, "has_onload": false, "pass": false }
 */

import ts from 'typescript';
import path from 'path';
import fs from 'fs';

const dir = process.argv[2];
const entryFile = process.argv[3] || 'index.js';

if (!dir) {
    process.stdout.write(JSON.stringify({ error: 'missing <dir> argument', has_onload: false, pass: false }));
    process.exit(1);
}

// ---------------------------------------------------------------------------
// onload 检测
// ---------------------------------------------------------------------------

/**
 * 递归遍历 TypeScript AST，查找名为 onload 的方法声明。
 * 返回 { line, column }（1-based）或 null（未找到）。
 * @param {ts.Node} node
 * @param {ts.SourceFile} sourceFile
 * @returns {{ line: number, column: number } | null}
 */
function findOnloadMethod(node, sourceFile) {
    if (ts.isMethodDeclaration(node)) {
        const name = node.name;
        if (ts.isIdentifier(name) && name.text === 'onload') {
            const { line, character } = sourceFile.getLineAndCharacterOfPosition(node.getStart(sourceFile));
            return { line: line + 1, column: character + 1 };
        }
    }
    let result = null;
    ts.forEachChild(node, (child) => {
        if (!result) result = findOnloadMethod(child, sourceFile);
    });
    return result;
}

// ---------------------------------------------------------------------------
// 构建 TypeScript 程序
// ---------------------------------------------------------------------------

try {
    let fileNames;
    let compilerOptions = {
        target: ts.ScriptTarget.Latest,
        allowJs: true,
        noEmit: true,
        skipLibCheck: true,
    };

    const tsConfigPath = path.join(dir, 'tsconfig.json');
    if (fs.existsSync(tsConfigPath)) {
        const configFile = ts.readConfigFile(tsConfigPath, ts.sys.readFile);
        if (!configFile.error) {
            const parsed = ts.parseJsonConfigFileContent(configFile.config, ts.sys, dir);
            if (parsed.fileNames.length > 0) {
                fileNames = parsed.fileNames;
                compilerOptions = { ...compilerOptions, ...parsed.options };
            }
        }
    }

    if (!fileNames || fileNames.length === 0) {
        // 回退：仅分析入口文件
        fileNames = [path.resolve(dir, entryFile)];
    }

    const program = ts.createProgram(fileNames, compilerOptions);

    // ---------------------------------------------------------------------------
    // 遍历所有源文件（跳过声明文件和 node_modules）
    // ---------------------------------------------------------------------------
    for (const sourceFile of program.getSourceFiles()) {
        if (sourceFile.isDeclarationFile) continue;
        const fn = sourceFile.fileName;
        if (fn.split(/[\/\\]/).includes('node_modules')) continue;

        const location = findOnloadMethod(sourceFile, sourceFile);
        if (location) {
            const relPath = path.relative(dir, fn).replace(/\\/g, '/');
            process.stdout.write(JSON.stringify({
                has_onload: true,
                pass: true,
                onload_file: relPath,
                onload_line: location.line,
                onload_column: location.column,
            }));
            process.exit(0);
        }
    }

    process.stdout.write(JSON.stringify({ has_onload: false, pass: false }));
} catch (err) {
    process.stdout.write(JSON.stringify({ error: err.message, has_onload: false, pass: false }));
    process.exit(1);
}
