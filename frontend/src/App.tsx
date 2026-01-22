import React, { useMemo, useState } from 'react';
import styles from './App.module.css';
import HealthTab from './components/HealthTab';
import PredictTab from './components/PredictTab';
import EvaluateTab from './components/EvaluateTab';
import EvaluateDbTab from './components/EvaluateDbTab';
import MatchCompareDbTab from './components/MatchCompareDbTab';
import OpsStatusTab from './components/OpsStatusTab';

type TabKey = 'health' | 'predict' | 'evaluate' | 'evaluateDb' | 'compareDb' | 'ops';

type TabButtonProps = {
  id: TabKey;
  active: boolean;
  onClick: React.Dispatch<React.SetStateAction<TabKey>>;
  children?: React.ReactNode;
};

const TabButton: React.FC<TabButtonProps> = ({ id, active, onClick, children }) => (
  <button
    onClick={() => onClick(id)}
    className={active ? `${styles.tabButton} ${styles.tabButtonActive}` : styles.tabButton}
  >
    {children}
  </button>
);

const App: React.FC = () => {
  const [tab, setTab] = useState<TabKey>('health');
  const baseUrl = useMemo(() => import.meta.env.VITE_ML_SERVICE_URL || 'http://localhost:8000', []);

  return (
    <div className={styles.container}>
      <h1 className={styles.title}>ML Control Panel</h1>
      <div className={styles.info}>
        <small>ML Service: {baseUrl}</small>
      </div>
      <div className={styles.tabs}>
        <TabButton id="health" active={tab === 'health'} onClick={setTab}>
          Health
        </TabButton>
        <TabButton id="ops" active={tab === 'ops'} onClick={setTab}>
          Ops Status
        </TabButton>
        <TabButton id="predict" active={tab === 'predict'} onClick={setTab}>
          Single Prediction
        </TabButton>
        <TabButton id="evaluate" active={tab === 'evaluate'} onClick={setTab}>
          Evaluate From CSV
        </TabButton>
        <TabButton id="evaluateDb" active={tab === 'evaluateDb'} onClick={setTab}>
          Evaluate (DB)
        </TabButton>
        <TabButton id="compareDb" active={tab === 'compareDb'} onClick={setTab}>
          Match Compare (DB)
        </TabButton>
      </div>

      {tab === 'health' && <HealthTab />}
      {tab === 'ops' && <OpsStatusTab />}
      {tab === 'predict' && <PredictTab />}
      {tab === 'evaluate' && <EvaluateTab />}
      {tab === 'evaluateDb' && <EvaluateDbTab />}
      {tab === 'compareDb' && <MatchCompareDbTab />}
    </div>
  );
};

export default App;
