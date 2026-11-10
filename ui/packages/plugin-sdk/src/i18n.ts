import type { TFunction, TFunctionDetailedResult, TOptions } from 'i18next'
import { useTranslation as useReactTranslation } from 'react-i18next'

import type {
  PluginTranslationDictionary,
  PluginTranslations,
} from './index.js'
import { usePlugin } from './navigation.js'

export type PluginTranslationKey<Dictionary> = string extends keyof Dictionary
  ? string
  : {
      [Key in keyof Dictionary & string]: Dictionary[Key] extends string
        ? Key
        : `${Key}.${PluginTranslationKey<Dictionary[Key]>}`
    }[keyof Dictionary & string]

type WithPluralKey<Key extends string> =
  Key extends `${infer Base}_ordinal_${Intl.LDMLPluralRule}`
    ? Key | Base
    : Key extends `${infer Base}_${Intl.LDMLPluralRule}`
      ? Key | Base
      : Key

type TranslationResult<Options extends TOptions> = Options extends {
  returnDetails: true
}
  ? TFunctionDetailedResult<string, Options>
  : string

type PluginTFunction<Key extends string> = {
  <const Options extends TOptions>(
    key: Key | Key[],
    options?: Options
  ): TranslationResult<Options>
  <const Options extends TOptions>(
    key: Key | Key[],
    defaultValue: string,
    options?: Options
  ): TranslationResult<Options>
}

function readLabel(
  dictionary: PluginTranslationDictionary,
  key: string
): string | undefined {
  const value = dictionary[key]
  if (typeof value === 'string') return value
  const [segment, ...rest] = key.split('.')
  const nested = dictionary[segment]
  return rest.length > 0 && typeof nested === 'object'
    ? readLabel(nested, rest.join('.'))
    : undefined
}

export function createPluginI18n<
  const Dictionary extends PluginTranslationDictionary,
>(resources: PluginTranslations & { en: Dictionary }) {
  return {
    resources,
    label(key: PluginTranslationKey<Dictionary>) {
      const labels: { en: string; [language: string]: string | undefined } = {
        en: readLabel(resources.en, key) as string,
      }
      for (const [language, messages] of Object.entries(resources)) {
        if (language !== 'en' && messages) {
          labels[language] = readLabel(messages, key)
        }
      }
      return labels
    },
    useTranslation() {
      const { pluginId } = usePlugin()
      const { t, i18n, ready } = useReactTranslation(
        `mplatform-plugin-${pluginId}`
      )
      return {
        t: t as TFunction as unknown as PluginTFunction<
          WithPluralKey<PluginTranslationKey<Dictionary>>
        >,
        i18n,
        ready,
        language: i18n.resolvedLanguage ?? i18n.language,
      }
    },
  }
}
