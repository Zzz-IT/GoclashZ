export type SupportedLocale = 'zh-CN' | 'zh-TW' | 'zh-HK' | 'en-US' | 'ja-JP' | 'ru-RU' | 'fr-FR' | 'de-DE';

export interface LanguageOption {
  label: string;
  value: SupportedLocale;
  description?: string;
}

export const SUPPORTED_LOCALES: LanguageOption[] = [
  { label: '简体中文', value: 'zh-CN' },
  { label: '繁體中文（台灣）', value: 'zh-TW' },
  { label: '繁體中文（香港）', value: 'zh-HK' },
  { label: 'English', value: 'en-US' },
  { label: '日本語', value: 'ja-JP' },
  { label: 'Русский', value: 'ru-RU' },
  { label: 'Français', value: 'fr-FR' },
  { label: 'Deutsch', value: 'de-DE' },
];

