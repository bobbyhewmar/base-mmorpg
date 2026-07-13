import './styles.css';

import { ClientApp } from './app/clientApp';

const app = document.querySelector<HTMLDivElement>('#app');

if (!app) {
  throw new Error('App root was not found.');
}

new ClientApp(app);
