export type Lang = 'en' | 'es' // English, and Spanish as spoken in Spain

const en = {
  // page
  title: 'WoW Auction Prices',
  tagline: 'Price history and buy/sell signals, net of the auction house cut.',
  error: 'Something went wrong. Please try again.',
  language: 'Language',
  updated: 'Prices updated {time} · {ago}',
  footer:
    'Data provided by Blizzard Entertainment. This site is not affiliated with or endorsed by Blizzard Entertainment.',
  // search
  ruleset: 'Ruleset',
  faction: 'Faction',
  category: 'Category',
  allCategories: 'All categories',
  searchItems: 'Search items',
  searchPlaceholder: 'e.g. linen cloth',
  search: 'Search',
  factionAlliance: 'Alliance',
  factionHorde: 'Horde',
  factionBlackwater: 'Blackwater (neutral)',
  // front page and results
  noData:
    'No auction data yet. The tracker keeps checking, and rulesets appear here as soon as Blizzard serves their auctions.',
  welcome: 'Welcome',
  heroTitle: 'Find your fortune in Azeroth',
  heroText: 'Search for an item to see its price history, e.g. “linen cloth”, or pick a category to browse.',
  browseByCategory: 'Browse by category',
  categoriesRanked: 'Categories are ranked by units sold in the last 7 days.',
  searching: 'Searching…',
  noItems: 'No items found. Try another name.',
  mostListedIn: 'Most listed in {name}',
  thisCategory: 'this category',
  colItem: 'Item',
  colPrice: 'Price',
  colSoldLow: 'Sold low',
  colSoldHigh: 'Sold high',
  hintSoldLow: 'Units sold in the last 7 days that cannot have simply expired',
  hintSoldHigh: 'Every unit that left in the last 7 days, including listings that may have expired',
  soldCount: '{n} sold',
  // details
  detailsOf: '{name} details',
  closeDetails: 'Close details',
  lowestNow: 'Lowest now',
  lastPriceSeen: 'Last price seen',
  notListedNow: 'Not listed now',
  timeRange: 'Time range',
  range24h: '24 hours',
  range7d: '7 days',
  range30d: '30 days',
  loadingHistory: 'Loading price history…',
  // money and time
  coinGold: 'g',
  coinSilver: 's',
  coinCopper: 'c',
  na: 'n/a',
  agoNow: 'just now',
  agoMinutes: '{n} min ago',
  agoHours: '{n} h ago',
  agoDays: '{n} d ago',
  // insights
  insights: 'Insights',
  verdictGoodBuy: 'Good time to buy',
  verdictGoodSell: 'Good time to sell',
  verdictNeutral: 'Fair price',
  verdictNoData: 'Not enough history yet',
  marketValue: 'Market value',
  lowestVsMarket: 'Lowest vs market value',
  cheaperThan: 'Cheaper than',
  cheaperThanValue: '{pct} of recent prices',
  change1d: 'Change (1 day)',
  change7d: 'Change (7 days)',
  supplyVsAvg: 'Supply vs average',
  demand: 'Demand',
  demandHigh: 'High demand',
  demandMedium: 'Medium demand',
  demandLow: 'Low demand',
  demandUnknown: 'Not enough data yet',
  measuring:
    'Measuring sales: they are worked out between two hourly snapshots, so the first figures appear after the next hourly refresh.',
  soldPerDay: 'Sold per day',
  daysOfSupply: 'Days of supply',
  listingsSold: 'Listings sold',
  newlyListed: 'Newly listed',
  unitsOf: '{range} units',
  daysOf: '{range} days',
  daysAtLeast: '{n}+ days',
  hoursOne: '{n} hour',
  hoursMany: '{n} hours',
  snapshotsOne: '{n} snapshot',
  snapshotsMany: '{n} snapshots',
  demandBasis:
    'Based on {hours} of snapshots. Blizzard does not publish sales or buyers: these figures are estimated from listings that disappeared between hourly snapshots. Cancelled and relisted auctions count as sales, so the higher number is an upper bound.',
  afterCut: 'After the {cut} auction house cut',
  youGetMarket: 'You get per unit selling at market value',
  matchLowest: 'Match the lowest price: you get per unit',
  nearMarket: 'Price near the market: you get per unit',
  commodityNote:
    'Orders fill cheapest first and a tie sells the newest listing first, so matching the lowest puts your stack in front; undercutting by 1c only loses money.',
  singleNote: 'One cheap listing is not the market: price against the wall of listings, not the lowest one.',
  breakEven: 'Break-even listing price if you buy at the lowest',
  flipMargin: 'Flip margin per unit (buy lowest, sell at market)',
  vendorPays: 'Vendor pays',
  noVendor: 'Vendors do not buy this item.',
  vendorNote:
    'A vendor pays more than the auction house would: after the cut you would get {net}, a vendor pays {vendor}. Sell it to a vendor.',
  mailNote: 'Gold from sales arrives by mail, up to about an hour later.',
  depositTitle: 'Deposit if the auction expires (estimate)',
  depositAria: 'Deposit if the auction expires',
  depositHours: '{n} hours',
  perItem: '{money} per item',
  depositNote:
    'Refunded if it sells, lost if it expires or you cancel. An estimate from the Classic rule (vendor price × deposit rate per 2 hours, at least 1c): unconfirmed for Forever, so check the Deposit line in the game when you post.',
  insightsBasis:
    'Based on {count}. Statistics from past prices, not financial advice. The margins above do not include deposits.',
  // investment planner
  planTitle: 'Investment planner',
  planIntro:
    'Enter the gold you want to invest and see the ten best items in high demand to buy and sell again, after the auction house cut.',
  planBudget: 'Gold to invest',
  planBudgetPlaceholder: 'e.g. 500',
  planGo: 'Find the best buys',
  planInvalid: 'Enter an amount of gold greater than zero and no more than 214,748.',
  planLoading: 'Looking for the best buys…',
  planNone: 'Nothing is worth buying with this budget right now.',
  planNoneWhy:
    'The planner only looks at items in high demand and needs several hours of price history and measured sales, so try again later or with another budget.',
  planColBuy: 'Buy at',
  planColSell: 'Sell at',
  planColUnits: 'Units',
  planColCost: 'Cost',
  planColProfit: 'Profit',
  planColReturn: 'Return',
  planColRisk: 'Risk',
  planAfterCut: 'after the cut: {net}',
  planSummary: 'Spend {spent} of {budget} and earn about {profit} after the {cut} cut ({ret} return).',
  planLeft: 'Left over: {amount}',
  riskLow: 'Low',
  riskMedium: 'Medium',
  riskHigh: 'High',
  planDays: 'About {days} days to sell',
  planNote:
    'Estimates from past prices, not financial advice. Prices change while you buy and while you wait to sell, so check the price in the game before buying a lot. Deposits are not counted (they come back if the auction sells). Risk is how often the market paid less than you need to break even after the cut, plus how long selling takes.',
  // chart
  noHistory: 'No price history yet.',
  chartFigure: 'Price history. Use the left and right arrow keys to read each snapshot.',
  chartTitle: 'Price history',
  chartSummary: 'Lowest price from {from} to {to} across {n} snapshots, latest {latest}.',
  medianPrice: 'Median price',
  lowestPrice: 'Lowest price',
  readout: 'Lowest {min}, median {median}, {n} listed',
  timesIn: 'Times in {zone}',
  localTime: 'your local time',
}

export type Key = keyof typeof en

// Spanish as spoken in Spain. The game's coin letters in Spanish are oro, plata and cobre.
const es: Record<Key, string> = {
  title: 'Precios de subastas de WoW',
  tagline: 'Historial de precios y señales de compra y venta, descontada la comisión de la casa de subastas.',
  error: 'Algo ha salido mal. Inténtalo de nuevo.',
  language: 'Idioma',
  updated: 'Precios actualizados a las {time} · {ago}',
  footer:
    'Datos proporcionados por Blizzard Entertainment. Este sitio no está afiliado a Blizzard Entertainment ni cuenta con su respaldo.',
  ruleset: 'Conjunto de reglas',
  faction: 'Facción',
  category: 'Categoría',
  allCategories: 'Todas las categorías',
  searchItems: 'Buscar objetos',
  searchPlaceholder: 'p. ej., paño de lino',
  search: 'Buscar',
  factionAlliance: 'Alianza',
  factionHorde: 'Horda',
  factionBlackwater: 'Blackwater (neutral)',
  noData:
    'Aún no hay datos de subastas. El rastreador sigue comprobando y los conjuntos de reglas aparecerán aquí en cuanto Blizzard sirva sus subastas.',
  welcome: 'Bienvenida',
  heroTitle: 'Encuentra tu fortuna en Azeroth',
  heroText:
    'Busca un objeto para ver su historial de precios, por ejemplo «paño de lino», o elige una categoría para explorar.',
  browseByCategory: 'Explorar por categoría',
  categoriesRanked: 'Las categorías están ordenadas por unidades vendidas en los últimos 7 días.',
  searching: 'Buscando…',
  noItems: 'No se ha encontrado ningún objeto. Prueba con otro nombre.',
  mostListedIn: 'Más listados en {name}',
  thisCategory: 'esta categoría',
  colItem: 'Objeto',
  colPrice: 'Precio',
  colSoldLow: 'Vendidas (mín.)',
  colSoldHigh: 'Vendidas (máx.)',
  hintSoldLow: 'Unidades vendidas en los últimos 7 días que no pueden haber caducado sin más',
  hintSoldHigh: 'Todas las unidades que han salido en los últimos 7 días, incluidas subastas que pueden haber caducado',
  soldCount: '{n} vendidas',
  detailsOf: 'Detalles de {name}',
  closeDetails: 'Cerrar detalles',
  lowestNow: 'Más barato ahora',
  lastPriceSeen: 'Último precio visto',
  notListedNow: 'No está a la venta ahora',
  timeRange: 'Intervalo de tiempo',
  range24h: '24 horas',
  range7d: '7 días',
  range30d: '30 días',
  loadingHistory: 'Cargando el historial de precios…',
  coinGold: 'o',
  coinSilver: 'p',
  coinCopper: 'c',
  na: 'n/d',
  agoNow: 'ahora mismo',
  agoMinutes: 'hace {n} min',
  agoHours: 'hace {n} h',
  agoDays: 'hace {n} d',
  insights: 'Análisis',
  verdictGoodBuy: 'Buen momento para comprar',
  verdictGoodSell: 'Buen momento para vender',
  verdictNeutral: 'Precio justo',
  verdictNoData: 'Aún no hay suficiente historial',
  marketValue: 'Valor de mercado',
  lowestVsMarket: 'Más barato frente al valor de mercado',
  cheaperThan: 'Más barato que',
  cheaperThanValue: '{pct} de los precios recientes',
  change1d: 'Variación (1 día)',
  change7d: 'Variación (7 días)',
  supplyVsAvg: 'Existencias frente a la media',
  demand: 'Demanda',
  demandHigh: 'Demanda alta',
  demandMedium: 'Demanda media',
  demandLow: 'Demanda baja',
  demandUnknown: 'Aún no hay suficientes datos',
  measuring:
    'Midiendo las ventas: se calculan entre dos capturas horarias, así que las primeras cifras aparecerán tras la próxima actualización horaria.',
  soldPerDay: 'Vendidas al día',
  daysOfSupply: 'Días de existencias',
  listingsSold: 'Subastas vendidas',
  newlyListed: 'Nuevas en subasta',
  unitsOf: '{range} unidades',
  daysOf: '{range} días',
  daysAtLeast: '{n}+ días',
  hoursOne: '{n} hora',
  hoursMany: '{n} horas',
  snapshotsOne: '{n} captura',
  snapshotsMany: '{n} capturas',
  demandBasis:
    'Basado en {hours} de capturas. Blizzard no publica ventas ni compradores: estas cifras se estiman a partir de las subastas que han desaparecido entre capturas horarias. Las subastas canceladas y vueltas a publicar cuentan como ventas, así que la cifra más alta es un límite superior.',
  afterCut: 'Tras la comisión del {cut} de la casa de subastas',
  youGetMarket: 'Recibes por unidad vendiendo al valor de mercado',
  matchLowest: 'Igualar el precio más bajo: recibes por unidad',
  nearMarket: 'Precio cercano al mercado: recibes por unidad',
  commodityNote:
    'Los pedidos se cumplen empezando por el más barato y, si hay empate, se vende antes la subasta más reciente; por eso igualar el precio más bajo pone tu pila por delante, y bajarlo 1 de cobre solo hace perder dinero.',
  singleNote: 'Una sola subasta barata no es el mercado: fija el precio según el conjunto de subastas, no según la más barata.',
  breakEven: 'Precio de venta para no perder si compras al más bajo',
  flipMargin: 'Margen de reventa por unidad (comprar al más bajo, vender al precio de mercado)',
  vendorPays: 'El vendedor paga',
  noVendor: 'Los vendedores no compran este objeto.',
  vendorNote:
    'Un vendedor paga más que la casa de subastas: tras la comisión recibirías {net}, y un vendedor paga {vendor}. Véndelo a un vendedor.',
  mailNote: 'El oro de las ventas llega por correo, hasta aproximadamente una hora después.',
  depositTitle: 'Depósito si la subasta caduca (estimación)',
  depositAria: 'Depósito si la subasta caduca',
  depositHours: '{n} horas',
  perItem: '{money} por objeto',
  depositNote:
    'Se devuelve si se vende y se pierde si caduca o la cancelas. Es una estimación según la regla de Classic (precio de vendedor × tasa de depósito por cada 2 horas, como mínimo 1 de cobre): no está confirmada para Forever, así que comprueba la línea de Depósito en el juego al publicar.',
  insightsBasis:
    'Basado en {count}. Estadísticas a partir de precios pasados, no asesoramiento financiero. Los márgenes anteriores no incluyen los depósitos.',
  planTitle: 'Planificador de inversión',
  planIntro:
    'Indica el oro que quieres invertir y mira los diez mejores objetos de alta demanda para comprar y volver a vender, descontada la comisión de la casa de subastas.',
  planBudget: 'Oro a invertir',
  planBudgetPlaceholder: 'p. ej., 500',
  planGo: 'Buscar las mejores compras',
  planInvalid: 'Indica una cantidad de oro mayor que cero y no superior a 214.748.',
  planLoading: 'Buscando las mejores compras…',
  planNone: 'Ahora mismo no merece la pena comprar nada con este presupuesto.',
  planNoneWhy:
    'El planificador solo mira objetos de alta demanda y necesita varias horas de historial de precios y ventas medidas, así que vuelve a intentarlo más tarde o con otro presupuesto.',
  planColBuy: 'Comprar a',
  planColSell: 'Vender a',
  planColUnits: 'Unidades',
  planColCost: 'Coste',
  planColProfit: 'Beneficio',
  planColReturn: 'Rentabilidad',
  planColRisk: 'Riesgo',
  planAfterCut: 'tras la comisión: {net}',
  planSummary: 'Gastas {spent} de {budget} y ganas unos {profit} tras la comisión del {cut} ({ret} de rentabilidad).',
  planLeft: 'Sobra: {amount}',
  riskLow: 'Bajo',
  riskMedium: 'Medio',
  riskHigh: 'Alto',
  planDays: 'Unos {days} días para venderlo',
  planNote:
    'Son estimaciones a partir de precios pasados, no asesoramiento financiero. Los precios cambian mientras compras y mientras esperas para vender, así que comprueba el precio en el juego antes de comprar mucho. No se cuentan los depósitos (se devuelven si la subasta se vende). El riesgo es cuántas veces el mercado pagó menos de lo que necesitas para no perder tras la comisión, más lo que tarda en venderse.',
  noHistory: 'Aún no hay historial de precios.',
  chartFigure: 'Historial de precios. Usa las flechas izquierda y derecha para leer cada captura.',
  chartTitle: 'Historial de precios',
  chartSummary: 'Precio más bajo de {from} a {to} en {n} capturas; el último, {latest}.',
  medianPrice: 'Precio mediano',
  lowestPrice: 'Precio más bajo',
  readout: 'Más bajo {min}, mediano {median}, {n} en subasta',
  timesIn: 'Horas en {zone}',
  localTime: 'tu hora local',
}

export const messages: { en: Record<Key, string>; es: Record<Key, string> } = { en, es }

export type Vars = Record<string, string | number>

/** The text for a key in a language, with {name} placeholders filled from vars. */
export function translate(lang: Lang, key: Key, vars: Vars = {}): string {
  return messages[lang][key].replace(/\{(\w+)\}/g, (whole, name: string) => (name in vars ? String(vars[name]) : whole))
}

/** The locale used for numbers and dates. */
export const localeOf = (lang: Lang) => (lang === 'es' ? 'es-ES' : 'en-US')

/** Spanish for any Spanish browser, English otherwise; the first language in the list is the one that counts. */
export function detectLang(languages: readonly string[]): Lang {
  return languages[0]?.toLowerCase().startsWith('es') ? 'es' : 'en'
}

const STORAGE_KEY = 'lang'

/** The language saved by a previous visit, else the browser's. */
export function initialLang(): Lang {
  try {
    const saved = localStorage.getItem(STORAGE_KEY)
    if (saved === 'en' || saved === 'es') return saved
  } catch {
    // storage can be blocked: fall back to the browser's language
  }
  return detectLang(navigator.languages?.length ? navigator.languages : [navigator.language])
}

export function saveLang(lang: Lang) {
  try {
    localStorage.setItem(STORAGE_KEY, lang)
  } catch {
    // not saving is fine
  }
}
