// @vitest-environment node

import {
  mkdirSync,
  mkdtempSync,
  readFileSync,
  rmSync,
  writeFileSync,
} from "node:fs";
import { createRequire } from "node:module";
import { tmpdir } from "node:os";
import path from "node:path";

import { Linter, type ESLint } from "eslint";
import { afterAll, beforeAll, describe, expect, it } from "vitest";

const require = createRequire(import.meta.url);
const nextRequire = createRequire(require.resolve("@next/eslint-plugin-next"));
const nextPlugin = require("@next/eslint-plugin-next") as ESLint.Plugin;
const { getRootDirs } =
  require("@next/eslint-plugin-next/dist/utils/get-root-dirs.js") as {
    getRootDirs(context: {
      cwd: string;
      settings: { next?: { rootDir: unknown } };
    }): string[];
  };
const guard = nextRequire("fast-glob") as {
  globSync(pattern: unknown, options: unknown): never;
};

describe("single-root Next ESLint guard", () => {
  let fixture: string;

  beforeAll(() => {
    fixture = mkdtempSync(path.join(tmpdir(), "arbion-next-glob-"));
    mkdirSync(path.join(fixture, "pages"));
    writeFileSync(
      path.join(fixture, "pages/index.js"),
      "export default () => null;",
    );
    writeFileSync(
      path.join(fixture, "pages/about.js"),
      "export default () => null;",
    );
  });

  afterAll(() => {
    if (fixture && path.basename(fixture).startsWith("arbion-next-glob-")) {
      rmSync(fixture, { recursive: true, force: true });
    }
  });

  it("binds only the exact reviewed plugin to a private dependency-free guard", () => {
    const manifest = JSON.parse(
      readFileSync(new URL("../package.json", import.meta.url), "utf8"),
    );
    expect(manifest.devDependencies["eslint-config-next"]).toBe("16.3.0");
    expect(manifest.overrides).toEqual({
      "@next/eslint-plugin-next@16.3.0": { "fast-glob": "$fast-glob" },
    });
    expect(require("@next/eslint-plugin-next/package.json").version).toBe(
      "16.3.0",
    );
    const packageFile = path.join(
      path.dirname(nextRequire.resolve("fast-glob")),
      "package.json",
    );
    const metadata = JSON.parse(readFileSync(packageFile, "utf8"));
    expect(metadata.name).toBe("@arbion/next-eslint-glob");
    expect(metadata.private).toBe(true);
    expect(metadata.dependencies).toBeUndefined();
    // Loading through Next's actual CommonJS require proves Node's import path,
    // not a separately imported module that might bypass the installed graph.
    expect(Object.keys(guard)).toEqual(["globSync"]);
  });

  it("removes the affected implementation rather than renaming its source", () => {
    const lock = readFileSync(
      new URL("../package-lock.json", import.meta.url),
      "utf8",
    );
    expect(lock).not.toMatch(
      /registry\.npmjs\.org\/(?:braces|micromatch|fast-glob)\//,
    );
    expect(lock).not.toMatch(
      /"(?:[^"\n]*\/)?node_modules\/(?:braces|micromatch)"/,
    );
  });

  it("keeps the composed repository config on the supported default-root path", async () => {
    const { default: config } = (await import(
      /* @vite-ignore */ new URL("../eslint.config.mjs", import.meta.url).href
    )) as { default: Linter.Config[] };
    expect(config.length).toBeGreaterThan(0);
    for (const entry of config) {
      const next = entry.settings?.next as Record<string, unknown> | undefined;
      expect(next && Object.hasOwn(next, "rootDir")).not.toBe(true);
    }
  });

  it("uses context.cwd without invoking the guard for default roots", () => {
    expect(getRootDirs({ cwd: fixture, settings: {} })).toEqual([fixture]);
  });

  it.each([
    ["literal", "/project"],
    ["glob", "/projects/*"],
    ["brace", "/projects/{one,two}"],
    ["array", ["/projects/one", "/projects/two"]],
    ["deeply nested pattern", "{".repeat(4000) + "root" + "}".repeat(4000)],
  ])("fails explicitly for unsupported %s custom roots", (_name, rootDir) => {
    expect(() =>
      getRootDirs({ cwd: fixture, settings: { next: { rootDir } } }),
    ).toThrow(/custom.*root|root.*custom/i);
  });

  it.each([undefined, null, "", [], "project"])(
    "never silently returns an empty result for direct guard input %j",
    (pattern) => {
      expect(() => guard.globSync(pattern, { onlyDirectories: true })).toThrow(
        /custom.*root|root.*custom/i,
      );
    },
  );

  it("still reports actual Next navigation and image violations at the default root", () => {
    const linter = new Linter({ cwd: fixture });
    const config: Linter.Config = {
      files: ["**/*.jsx"],
      languageOptions: {
        parserOptions: {
          ecmaVersion: "latest",
          sourceType: "module",
          ecmaFeatures: { jsx: true },
        },
      },
      plugins: { "@next/next": nextPlugin },
      rules: {
        "@next/next/no-html-link-for-pages": "error",
        "@next/next/no-img-element": "error",
      },
    };
    const messages = linter.verify(
      'export default () => <><a href="/about">About</a><img src="/logo.png" alt="Logo" /></>;',
      config,
      { filename: path.join(fixture, "fixture.jsx") },
    );
    expect(messages.map((message) => message.ruleId).sort()).toEqual([
      "@next/next/no-html-link-for-pages",
      "@next/next/no-img-element",
    ]);
    expect(
      messages.every((message) => message.severity === 2 && !message.fatal),
    ).toBe(true);
    expect(
      linter.verify(
        'export default () => <a href="https://example.com">External</a>;',
        config,
        { filename: path.join(fixture, "fixture.jsx") },
      ),
    ).toEqual([]);
  });
});
