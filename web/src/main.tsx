import React from 'react';
import ReactDOM from 'react-dom/client';
import App from './App';
import './styles.css';

const theme = localStorage.getItem('agentplane.theme') || 'light';
document.documentElement.setAttribute('data-theme', theme);

ReactDOM.createRoot(document.getElementById('root')!).render(
  <React.StrictMode>
    <App />
  </React.StrictMode>
);
