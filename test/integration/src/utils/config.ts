declare const import_meta_env_VITE_API_BASE: string;
declare const import_meta_env_MODE: string;

export function getApiBase(): string {
  return import.meta.env.VITE_API_BASE;
}

export function getMode(): string {
  return import.meta.env.MODE;
}
