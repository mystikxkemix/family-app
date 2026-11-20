import { StrictMode } from 'react';
import { createRoot } from 'react-dom/client';
import { App } from './App';
import { I18nProvider } from './i18n';
import { SessionProvider } from './lib/session';
import { FeedbackProvider } from './components/Feedback';
import './styles.css';

createRoot(document.getElementById('root')!).render(
  <StrictMode>
    <I18nProvider>
      <FeedbackProvider>
        <SessionProvider>
          <App />
        </SessionProvider>
      </FeedbackProvider>
    </I18nProvider>
  </StrictMode>,
);
