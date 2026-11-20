import { createContext, useCallback, useContext, useMemo, useState, type ReactNode } from 'react';
import { en, type MessageKey } from './en';
import { fr } from './fr';

export type Language = 'en' | 'fr';
export type { MessageKey };

const dictionaries: Record<Language, Record<MessageKey, string>> = { en, fr };
const storageKey = 'family.language';

/** Language saved by the user, else the browser's (French when it starts with "fr"). */
function initialLanguage(): Language {
  try {
    const saved = localStorage.getItem(storageKey);
    if (saved === 'en' || saved === 'fr') return saved;
  } catch {
    // Storage can be blocked (private mode): fall back to the browser language.
  }
  return navigator.language.toLowerCase().startsWith('fr') ? 'fr' : 'en';
}

type Params = Record<string, string | number>;

interface I18n {
  lang: Language;
  /** BCP 47 locale for Intl (dates, numbers, lists). */
  locale: string;
  setLang: (lang: Language) => void;
  /** Text for a key, with "{name}" placeholders filled from params. */
  t: (key: MessageKey, params?: Params) => string;
  /** Plural text: uses "<base>.one" or "<base>.other", with {count}. */
  tp: (base: string, count: number) => string;
}

const I18nContext = createContext<I18n | null>(null);

export function I18nProvider({ children }: { children: ReactNode }) {
  const [lang, setLangState] = useState<Language>(initialLanguage);

  const setLang = useCallback((next: Language) => {
    setLangState(next);
    try {
      localStorage.setItem(storageKey, next);
    } catch {
      // Not saved: the choice only lasts for this visit.
    }
  }, []);

  const value = useMemo<I18n>(() => {
    const dictionary = dictionaries[lang];
    const locale = lang === 'fr' ? 'fr-FR' : 'en-US';
    const t = (key: MessageKey, params?: Params) => {
      let text = dictionary[key] ?? en[key] ?? key;
      if (params) {
        for (const [name, value] of Object.entries(params)) {
          text = text.split(`{${name}}`).join(String(value));
        }
      }
      return text;
    };
    const plural = new Intl.PluralRules(locale);
    const tp = (base: string, count: number) => {
      const form = plural.select(count) === 'one' ? 'one' : 'other';
      return t(`${base}.${form}` as MessageKey, { count });
    };
    document.documentElement.lang = lang;
    return { lang, locale, setLang, t, tp };
  }, [lang, setLang]);

  return <I18nContext.Provider value={value}>{children}</I18nContext.Provider>;
}

export function useI18n(): I18n {
  const context = useContext(I18nContext);
  if (!context) throw new Error('useI18n must be used inside I18nProvider');
  return context;
}
