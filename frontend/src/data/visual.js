// Datos fijos de la interfaz: fotos de portada, categorías del inicio, etiquetas y zonas.
// No son datos del negocio (esos vienen del backend por api/ghostkitchen.js): son imágenes y
// textos de la pantalla. Las zonas son barrios con su centro aproximado, para sugerir la sede
// más cercana a donde está el cliente.

const img = (id, w = 900) => `https://images.unsplash.com/photo-${id}?auto=format&fit=crop&w=${w}&q=80`;

export const photos = {
  burger: "/images/platos/p1-mega-doble.webp",
  burger2: "https://images.unsplash.com/photo-1596649299486-4cdea56fd59d?auto=format&fit=crop&w=900&q=85",
  burger3: img("1568901346375-23c9450c58cd"),
  burger4: img("1571091718767-18b5b1457add"),
  burger5: img("1572802419224-296b0aeee0d9"),
  burgerPapas: img("1594212699903-ec8a3eca50f5"),
  pizza: "https://images.unsplash.com/photo-1589010588553-46e8e7c21788?auto=format&fit=crop&w=900&q=85",
  pizza2: img("1513104890138-7c749659a591"),
  pizza3: img("1565299624946-b28f40a0ae38"),
  veggie: "https://images.unsplash.com/photo-1597958792579-bd3517df6399?auto=format&fit=crop&w=900&q=85",
  bowl: img("1512621776951-a57141f2eefd"),
  poke: img("1546069901-ba9599a7e63c"),
  ensalada: img("1540189549336-e6e99c3679fe"),
  quinoa: img("1547592180-85f173990554"),
  feast: "https://images.unsplash.com/photo-1700513971603-eda40374ba0a?auto=format&fit=crop&w=1200&q=85",
  sushi: "https://images.unsplash.com/photo-1576866206724-c696f4c9fa06?auto=format&fit=crop&w=900&q=85",
  sushi2: img("1579871494447-9811cf80d66c"),
  sushiBarco: img("1553621042-f6e147245754"),
  sushiTabla: img("1611143669185-af224c5e3252"),
  nigiri: img("1617196034796-73dfa7b1fd56"),
  penne: img("1621996346565-e3dbc646d9a9"),
  spaghetti: img("1563379926898-05f4575a45d8"),
  carbonara: img("1588013273468-315fd88ea34c"),
  milanesa: "/images/platos/p34-napolitana-con-papas.webp",
  polloFrito: "/images/platos/p31-balde-de-pollo-x-8.webp",
  bife: img("1600891964092-4316c288032e"),
  asado: img("1529692236671-f1f6cf9683ba"),
  donas: "/images/platos/p36-donas-glaseadas-x-3.webp",
  vasitos: img("1488477181946-6428a0291777"),
  torta: img("1578985545062-69928b1d9587"),
  cookies: img("1499636136210-6f4ee915583e"),
  batido: img("1563805042-7684c019e1cb"),
  cafe: "/images/platos/p38-cafa-con-leche.webp",
  cafe2: img("1495474472287-4d71bcdd2085"),
  local: img("1555396273-367ea4eb4db5", 1200),
};

// Accesos rápidos del inicio. "id" es el valor EXACTO que tienen los platos en Catálogo (la
// búsqueda filtra por categoría exacta); "label" es lo que se muestra. Las etiquetas (esEtiqueta)
// filtran por restricción alimentaria en lugar de categoría.
export const categorias = [
  { id: "Hamburguesas", label: "Hamburguesas", foto: photos.burger3 },
  { id: "Pizzas", label: "Pizza", foto: photos.pizza2 },
  { id: "Sushi", label: "Sushi", foto: photos.sushi2 },
  { id: "Pastas", label: "Pastas", foto: photos.penne },
  { id: "Milanesas", label: "Milanesas", foto: photos.milanesa },
  { id: "Pollo", label: "Pollo", foto: photos.polloFrito },
  { id: "Carnes", label: "Carnes", foto: photos.bife },
  { id: "Vegano", label: "Vegano", foto: photos.bowl, esEtiqueta: true },
  { id: "Postres", label: "Postres", foto: photos.donas },
  { id: "Cafetería", label: "Cafetería", foto: photos.cafe },
  { id: "Sin TACC", label: "Sin TACC", foto: photos.poke, esEtiqueta: true },
];

export const ETIQUETAS = ["Vegano", "Vegetariano", "Sin TACC", "Picante"];

export const zonas = [
  { id: "nueva-cordoba", nombre: "Nueva Córdoba", lat: -31.4250, lng: -64.1870 },
  { id: "centro", nombre: "Centro", lat: -31.4167, lng: -64.1833 },
  { id: "guemes", nombre: "Güemes", lat: -31.4290, lng: -64.1960 },
  { id: "observatorio", nombre: "Observatorio", lat: -31.4235, lng: -64.1990 },
  { id: "alberdi", nombre: "Alberdi", lat: -31.4105, lng: -64.2045 },
  { id: "alto-alberdi", nombre: "Alto Alberdi", lat: -31.4060, lng: -64.2210 },
  { id: "paso-de-los-andes", nombre: "Paso de los Andes", lat: -31.4160, lng: -64.2010 },
  { id: "cofico", nombre: "Cofico", lat: -31.4000, lng: -64.1880 },
  { id: "alta-cordoba", nombre: "Alta Córdoba", lat: -31.3950, lng: -64.1800 },
  { id: "general-paz", nombre: "General Paz", lat: -31.4100, lng: -64.1650 },
  { id: "juniors", nombre: "Juniors", lat: -31.4180, lng: -64.1620 },
  { id: "san-vicente", nombre: "San Vicente", lat: -31.4255, lng: -64.1560 },
  { id: "ciudad-universitaria", nombre: "Ciudad Universitaria", lat: -31.4380, lng: -64.1900 },
  { id: "parque-velez-sarsfield", nombre: "Parque Vélez Sarsfield", lat: -31.4420, lng: -64.1950 },
  { id: "barrio-jardin", nombre: "Barrio Jardín", lat: -31.4500, lng: -64.1830 },
  { id: "jardin-espinosa", nombre: "Jardín Espinosa", lat: -31.4600, lng: -64.1760 },
  { id: "villa-el-libertador", nombre: "Villa El Libertador", lat: -31.4800, lng: -64.2000 },
  { id: "cerro-de-las-rosas", nombre: "Cerro de las Rosas", lat: -31.3700, lng: -64.2300 },
  { id: "cerro-chico", nombre: "Cerro Chico", lat: -31.3650, lng: -64.2400 },
  { id: "urca", nombre: "Urca", lat: -31.3850, lng: -64.2150 },
  { id: "poeta-lugones", nombre: "Poeta Lugones", lat: -31.3760, lng: -64.2050 },
  { id: "marques-de-sobremonte", nombre: "Marqués de Sobremonte", lat: -31.3720, lng: -64.1750 },
  { id: "villa-belgrano", nombre: "Villa Belgrano", lat: -31.3500, lng: -64.2600 },
  { id: "granja-de-funes", nombre: "Granja de Funes", lat: -31.3400, lng: -64.2350 },
  { id: "arguello", nombre: "Argüello", lat: -31.3330, lng: -64.2550 },
  { id: "valle-escondido", nombre: "Valle Escondido", lat: -31.3720, lng: -64.2850 },
  { id: "villa-rivera-indarte", nombre: "Villa Rivera Indarte", lat: -31.3250, lng: -64.2700 },
  { id: "villa-allende", nombre: "Villa Allende", lat: -31.2950, lng: -64.2950 },
  { id: "mendiolaza", nombre: "Mendiolaza", lat: -31.2650, lng: -64.3000 },
  { id: "saldan", nombre: "Saldán", lat: -31.3050, lng: -64.3150 },
  { id: "unquillo", nombre: "Unquillo", lat: -31.2330, lng: -64.3170 },
];

// Foto de portada de cada cocina, por id. Catálogo no guarda fotos de las cocinas.
export const FOTOS_COCINA = {
  "nueva-cordoba": photos.feast,
  cerro: photos.local,
  "villa-allende": photos.burger2,
  "barrio-jardin": photos.sushiTabla,
};

// Una cocina tiene como máximo 4 restaurantes. Catálogo lo controla igual; acá solo
// sirve para desactivar el botón antes de intentarlo.
export const MAX_RESTAURANTES = 4;
