import { describe, expect, it } from "vitest";

import { asList } from "./response";

describe("automation API collections", () => {
  it("treats null and missing collections as empty", () => {
    expect(asList(null)).toEqual([]);
    expect(asList(undefined)).toEqual([]);
    expect(asList([{ id: "automation-1" }])).toEqual([{ id: "automation-1" }]);
  });
});
