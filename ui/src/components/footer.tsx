import { useTranslation } from 'react-i18next'

const CURRENT_YEAR = new Date().getFullYear()

export function Footer() {
  const { t } = useTranslation()
  return (
    <footer className="border-t">
      <div className="container mx-auto px-4 py-6">
        <div className="flex flex-col md:flex-row justify-between items-center space-y-2 md:space-y-0">
          <p className="text-sm text-muted-foreground">
            {t('login.footer', { year: CURRENT_YEAR })}
          </p>
          <div className="flex space-x-6 text-sm text-muted-foreground">
            <a
              href="https://github.com/qzytwway/mplatform"
              target="_blank"
              className="app-link"
            >
              GitHub
            </a>
          </div>
        </div>
      </div>
    </footer>
  )
}
