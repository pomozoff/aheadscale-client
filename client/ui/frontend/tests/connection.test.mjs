import { test } from "node:test";
import assert from "node:assert/strict";
import { readFile } from "node:fs/promises";
import vm from "node:vm";
import ts from "typescript";

const source = await readFile(new URL("../src/lib/connection.ts", import.meta.url), "utf8");
const code = ts.transpileModule(source, { compilerOptions: { module: ts.ModuleKind.ESNext, target: ts.ScriptTarget.ES2022 } }).outputText;
const deferred = () => Promise.withResolvers();

// Только внешние Wails IPC и диалоги заменены: выполняется настоящий connection.ts.
async function fixture(overrides = {}) {
    const handlers = new Set();
    const dialogs = [];
    const login = deferred();
    let up = 0, wait = 0;
    const context = vm.createContext({ console, DOMException });
    const dependencies = {
        "@wailsio/runtime": { Events: { On: (_, handler) => { handlers.add(handler); return () => handlers.delete(handler); } } },
        "@bindings/services": {
            Connection: { Login: () => login.promise, Up: async () => { up++; }, WaitSSOLoginAndUp: async () => { wait++; }, ...overrides },
            WindowManager: { OpenBrowserLogin: async () => {}, CloseBrowserLogin: async () => {} },
        },
        "@/lib/i18n": { default: { t: (key) => key } },
        "@/lib/errors.ts": { errorDialog: async (dialog) => dialogs.push(dialog), formatErrorMessage: String },
    };
    const module = new vm.SourceTextModule(code, { context });
    await module.link(async (name) => {
        assert.ok(name in dependencies, `unexpected import ${name}`);
        const exports = dependencies[name];
        return new vm.SyntheticModule(Object.keys(exports), function () {
            for (const [name, value] of Object.entries(exports)) this.setExport(name, value);
        }, { context });
    });
    await module.evaluate();
    return { start: module.namespace.startConnection, login, dialogs, handlers,
        cancel: () => { for (const handler of handlers) handler(); },
        calls: () => ({ up, wait }) };
}

const result = { needsSsoLogin: false, verificationUri: "", verificationUriComplete: "", userCode: "", profileId: "default" };

test("Cancel during native Login suppresses cancellation error and removes listeners", async () => {
    const f = await fixture();
    const pending = f.start();
    f.cancel();
    f.login.reject(new Error("context canceled"));
    await pending;
    assert.deepEqual(f.dialogs, [], "Cancel must not open Login Failed");
    assert.deepEqual(f.calls(), { up: 0, wait: 0 });
    assert.equal(f.handlers.size, 0);
});

test("Cancel before Login resolves prevents a late Up", async () => {
    const f = await fixture();
    const pending = f.start();
    f.cancel();
    f.login.resolve(result);
    await pending;
    assert.deepEqual(f.calls(), { up: 0, wait: 0 });
    assert.equal(f.handlers.size, 0);
});

test("aborting while Login waits suppresses native error", async () => {
    const f = await fixture();
    const controller = new AbortController();
    const pending = f.start(undefined, controller.signal);
    controller.abort();
    f.login.reject(new Error("context canceled"));
    await assert.rejects(pending, { name: "AbortError" });
    assert.deepEqual(f.dialogs, []);
    assert.equal(f.handlers.size, 0);
});

test("successful Login still connects, real failures still open a dialog", async () => {
    const f = await fixture();
    const first = f.start();
    f.login.resolve(result);
    await first;
    assert.deepEqual(f.calls(), { up: 1, wait: 0 });
    assert.equal(f.handlers.size, 0);
    const failed = await fixture();
    const second = failed.start();
    failed.login.reject(new Error("native service unavailable"));
    await second;
    assert.equal(failed.dialogs.length, 1);
    assert.match(failed.dialogs[0].Message, /native service unavailable/);
});
