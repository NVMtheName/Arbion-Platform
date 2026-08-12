import nextVitals from "eslint-config-next/core-web-vitals";
import eslintConfigPrettier from "eslint-config-prettier";
import nextTypeScript from "eslint-config-next/typescript";

const config = [
  ...nextVitals,
  ...nextTypeScript,
  eslintConfigPrettier,
  { ignores: [".next/**", "coverage/**", "next-env.d.ts"] },
];

export default config;
