import http from 'k6/http';
import { check } from 'k6';
import { Rate, Trend } from 'k6/metrics';
import { SharedArray } from 'k6/data';
import papaparse from 'https://jslib.k6.io/papaparse/5.1.1/index.js';

// Métricas customizadas
const errorRate = new Rate('errors');
const cacheHitRate = new Rate('cache_hits');
const responseTime = new Trend('response_time');

// Carregar usuários do CSV uma única vez
const users = new SharedArray('users', function () {
  const csvData = open('./users.csv'); // Arquivo CSV deve estar na mesma pasta
  const parsedData = papaparse.parse(csvData, { 
    header: true, 
    skipEmptyLines: true 
  });
  
  console.log(`📁 Loaded ${parsedData.data.length} users from CSV`);
  return parsedData.data;
});

// Configuração do teste de carga
export let options = {
  stages: [
    // Ramp-up gradual
    { duration: '2m', target: 100 },   // 0-100 users em 2 min
    { duration: '3m', target: 500 },   // 100-500 users em 3 min  
    { duration: '5m', target: 1000 },  // 500-1000 users em 5 min
    // { duration: '10m', target: 2000 }, // 1000-2000 users em 10 min (pico)
    { duration: '5m', target: 1000 },  // Reduzir para 1000
    { duration: '3m', target: 0 },     // Ramp-down
  ],
  thresholds: {
    http_req_duration: ['p(90)<1000', 'p(95)<2000', 'p(99)<5000'], // P90 < 1s, P95 < 2s, P99 < 5s
    http_req_failed: ['rate<0.1'],     // Error rate < 10%
    errors: ['rate<0.05'],             // Error rate customizado < 5%
    cache_hits: ['rate>0.7'],          // Cache hit rate > 70%
  },
};

// Base URL da API
const BASE_URL = 'http://localhost:8888';

// Configuração dos cenários (ajuste os percentuais conforme necessário)
const SCENARIO_CONFIG = {
  BY_ID: { weight: 0.6, endpoint: 'v1/graph' },           // 60% - busca por ID
  BY_EMAIL: { weight: 0.3, endpoint: 'v1/graph/by-property/email/value' },    // 30% - busca por email  
  BY_CPF: { weight: 0.1, endpoint: 'v1/graph/by-property/document.value/value' }  // 10% - busca por CPF
};

export default function () {
  // Selecionar usuário aleatório
  const user = users[Math.floor(Math.random() * users.length)];
  
  // Determinar cenário baseado nos pesos
  const scenario = getRandomScenario();
  
  // Montar URL baseada no cenário e dados do usuário
  const url = buildUrl(scenario, user);
  
  // Executar request
  executeRequest(url, scenario, user);
}

function getRandomScenario() {
  const random = Math.random();
  let cumulative = 0;
  
  for (const [scenarioName, config] of Object.entries(SCENARIO_CONFIG)) {
    cumulative += config.weight;
    if (random <= cumulative) {
      return scenarioName;
    }
  }
  
  return 'BY_ID'; // fallback
}

function buildUrl(scenario, user) {
  const baseUrl = `${BASE_URL}/${SCENARIO_CONFIG[scenario].endpoint}`;
  
  // ADIÇÃO: Query parameters aleatórios para cache miss
  const depthLimit = Math.floor(Math.random() * 10) + 1; // 1-10
  const startTimes = ['2025-09-01', '2025-08-01', '2025-07-01', '2025-06-01', '2025-05-01', '2025-04-01'];
  const startTime = startTimes[Math.floor(Math.random() * startTimes.length)];
  
  const queryParams = `?depthLimit=${depthLimit}&startTime=${startTime}`;
  
  switch (scenario) {
    case 'BY_ID':
      return `${baseUrl}/${user.id}${queryParams}`;
    
    case 'BY_EMAIL':
      return `${baseUrl}/${user.email}${queryParams}`;
    
    case 'BY_CPF':
      return `${baseUrl}/${user.cpf}${queryParams}`;
    
    default:
      throw new Error(`Unknown scenario: ${scenario}`);
  }
}

function executeRequest(url, scenario, user) {
  const response = http.get(url, {
    headers: {
      'Content-Type': 'application/json',
      'User-Agent': 'k6-load-test',
    },
    timeout: '10s',
    tags: {
      scenario: scenario,
      user_id: user.id
    }
  });
  
  // Checks de validação (ajuste conforme sua API)
  const success = check(response, {
    'status is 200 or 404': (r) => r.status === 200 || r.status === 404,
    'response time < 5s': (r) => r.timings.duration < 5000,
    'response has valid format': (r) => {
      if (r.status !== 200) return true; // 404 é válido
      
      try {
        const json = JSON.parse(r.body);
        return json !== null && typeof json === 'object';
      } catch {
        return false;
      }
    },
  });
  
  // Detectar cache hit baseado no tempo de resposta
  // Ajuste os valores conforme sua infraestrutura
  let cacheThreshold;
  switch (scenario) {
    case 'BY_ID':
      cacheThreshold = 100; // ID queries são mais rápidas com cache
      break;
    case 'BY_EMAIL':
      cacheThreshold = 200; // Email queries um pouco mais lentas
      break;
    case 'BY_CPF':
      cacheThreshold = 300; // CPF queries podem ser mais complexas
      break;
  }
  
  const isCacheHit = response.timings.duration < cacheThreshold;
  cacheHitRate.add(isCacheHit);
  
  // Registrar métricas
  errorRate.add(!success);
  responseTime.add(response.timings.duration);
  
  // Log de debug para requests com problema (opcional)
  if (!success || response.timings.duration > 3000) {
    console.log(`⚠️  Slow/Failed request: ${scenario} ${user.id} - ${response.status} - ${response.timings.duration.toFixed(0)}ms`);
  }
}

// Função para setup inicial
export function setup() {
  console.log('🚀 Starting User Profile API Load Test (Cache Busting Edition)');
  console.log(`👥 Testing with ${users.length} users from CSV`);
  console.log(`📊 Scenarios: ID(${SCENARIO_CONFIG.BY_ID.weight*100}%) Email(${SCENARIO_CONFIG.BY_EMAIL.weight*100}%) CPF(${SCENARIO_CONFIG.BY_CPF.weight*100}%)`);
  console.log(`⏱️  Duration: ~30 minutes`);
  console.log(`🔥 NEW: Random depthLimit (1-10) + startTime parameters to break cache`);
  
  // Teste rápido com um usuário para validar endpoints
  console.log('🧪 Testing endpoints with sample user...');
  const sampleUser = users[0];
  
  const testUrls = [
    buildUrl('BY_ID', sampleUser),
    buildUrl('BY_EMAIL', sampleUser),
    buildUrl('BY_CPF', sampleUser)
  ];
  
  testUrls.forEach((url, index) => {
    const testResponse = http.get(url, { timeout: '5s' });
    const scenario = Object.keys(SCENARIO_CONFIG)[index];
    console.log(`${scenario}: ${url} -> ${testResponse.status} (${testResponse.timings.duration.toFixed(0)}ms)`);
  });
  
  return { startTime: Date.now() };
}

// Função para teardown
export function teardown(data) {
  const duration = (Date.now() - data.startTime) / 1000;
  console.log(`🏁 Load test completed in ${duration.toFixed(1)} seconds`);
  console.log('📈 Check the summary for detailed metrics');
  console.log('🔥 Expected: MUCH lower cache hits due to query parameter variations');
  console.log('💡 Compare with previous test results to see cache impact');
}