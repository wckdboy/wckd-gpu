import tseslint from "typescript-eslint";

export default tseslint.config(
  { ignores: ["dist/**", "dev-dist/**"] },
  ...tseslint.configs.recommended,
);
