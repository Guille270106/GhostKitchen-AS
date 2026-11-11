// Fotos de los platos de ejemplo (los del seed de Catálogo), por id de plato. Cada entrada
// guarda de dónde salió la foto. "plato" es el nombre del plato en el seed: sirve para revisar
// que la foto corresponda. Si un plato no está acá, la tarjeta muestra un recuadro sin foto:
// es mejor que una foto de otra comida. Si la cocina carga una foto (fotoUrl), se usa esa.
// Algunas fotos son ilustrativas: no todas pertenecen a la marca del menú. Las de Wikimedia Commons
// tienen licencia libre (CC0 o Creative Commons) y guardan "autor" y "licencia" para citarlas.
// Sin foto a propósito (no había una que mostrara ese plato): Veggie burger (la foto es una hamburguesa de carne); Pizza calabresa picante (la foto es una pizza de jamón y morrones); Sundae de dulce de leche (la foto es una copa de frutilla); Brownie con helado (la foto es una porción de torta, sin helado); Donas glaseadas (la foto es una berlinesa con dulce de leche); Iced latte (la foto es un café caliente).
export const dishPhotos = {
  "p1": {
    "plato": "Mega doble",
    "foto": "/images/platos/p1-mega-doble.webp",
    "fuente": "https://mcdonalds.es/productos/sandwiches-principales/cuarto-de-libra-doble",
    "imagenOriginal": "https://mcdonalds.es/api/cms/images/Z4mnfZbqstJ99jE1_Hero-Cuarto-de-Libra-Doble_desk.png?auto=compress&fm=webp&rect=524%2C0%2C873%2C679"
  },
  "p2": {
    "plato": "Cheese bacon",
    "foto": "/images/platos/p2-cheese-bacon.webp",
    "fuente": "https://barrio.ar/mostaza-burgers",
    "imagenOriginal": "https://cdn.myportfolio.com/165fa14f-ffc8-4ae0-92a9-c7ecb33f89c9/aa8dd416-8052-4cfb-ad2f-3cbc7798829d_rw_3840.jpg?h=5ebb18d4eda005998878f5b3e588df1d"
  },
  "p4": {
    "plato": "Papas medianas",
    "foto": "/images/platos/p30-papas-pons.webp",
    "fuente": "https://medium.com/%40vietkieutie/kill-with-kindness-and-a-plate-of-french-fries-b703e7708f34",
    "imagenOriginal": "https://miro.medium.com/v2/resize%3Afit%3A3340/1%2AuDZtJW4ZkWjEwUHjffrFWA.jpeg"
  },
  "p5": {
    "plato": "Salmón roll x 10",
    "foto": "/images/platos/p5-salma3n-roll-x-10.webp",
    "fuente": "https://www.sushipop.com.ar/menu",
    "imagenOriginal": "https://sushipopimg.s3.amazonaws.com/productos/carousel/Xatbbs-84BYJ2oKkPomo4xVM8lXwORdi.webp"
  },
  "p6": {
    "plato": "Philadelphia roll x 10",
    "foto": "/images/platos/p6-philadelphia-x-10.webp",
    "fuente": "https://buy.am/en/products/mao/classic-philadelphia-roll/mao054",
    "imagenOriginal": "https://buy.am/media/image/1d/c8/e8/Mao054.jpg"
  },
  "p7": {
    "plato": "Veggie roll x 10",
    "foto": "/images/platos/p7-roll-de-palta-x-10.webp",
    "fuente": "https://tasteofseoul.ca/products/avocu-roll",
    "imagenOriginal": "https://tasteofseoul.ca/cdn/shop/files/avocuroll_8pcs.jpg?v=1696206699"
  },
  "p8": {
    "plato": "Gyozas de cerdo x 6",
    "foto": "/images/platos/p8-gyozas-de-cerdo-x-6.webp",
    "fuente": "https://commons.wikimedia.org/wiki/File:Gyoza_(52769973678).jpg",
    "autor": "nakashi from Chofu, Tokyo, JAPAN",
    "licencia": "CC BY-SA 2.0"
  },
  "p9": {
    "plato": "Pizza muzzarella",
    "foto": "/images/platos/p9-muzzarella-grande.webp",
    "fuente": "https://la100.cienradios.com/curiosidades/cuanto-sale-una-pizza-de-muzzarella-grande-en-kentucky-en-agosto-2025/",
    "imagenOriginal": "https://la100.cienradios.com/resizer/v2/P377PSCARBBIPIBRSHRILC4JDY.jpg?auth=3a84ea7ff1547ac6b088eb342559f40949324a2f203e5f72a394576a55081e82&height=720&width=1440"
  },
  "p10": {
    "plato": "Pizza napolitana",
    "foto": "/images/platos/p10-pizza-napolitana.webp",
    "fuente": "https://commons.wikimedia.org/wiki/File:Pizza_IMG_0794_(39443271415).jpg",
    "autor": "N i c o l a from Fiumicino (Rome), Italy",
    "licencia": "CC BY 2.0"
  },
  "p12": {
    "plato": "Empanadas de carne x 6",
    "foto": "/images/platos/p12-empanadas-de-carne-x-6.webp",
    "fuente": "https://commons.wikimedia.org/wiki/File:Empanadas_salte%C3%B1as.jpg",
    "autor": "travelwayoflife",
    "licencia": "CC BY-SA 2.0"
  },
  "p13": {
    "plato": "Helado 1 kg",
    "foto": "/images/platos/p14-cuarto-de-helado.webp",
    "fuente": "https://cdn.pedix.app/49DPjhacOaWFpaSbwIwq/products/vBvqj1oriUzdrAxcVC1b-.png?size=1600x1600",
    "imagenOriginal": "https://cdn.pedix.app/49DPjhacOaWFpaSbwIwq/products/vBvqj1oriUzdrAxcVC1b-.png?size=1600x1600"
  },
  "p14": {
    "plato": "Helado 1/2 kg",
    "foto": "/images/platos/p14-cuarto-de-helado.webp",
    "fuente": "https://cdn.pedix.app/49DPjhacOaWFpaSbwIwq/products/vBvqj1oriUzdrAxcVC1b-.png?size=1600x1600",
    "imagenOriginal": "https://cdn.pedix.app/49DPjhacOaWFpaSbwIwq/products/vBvqj1oriUzdrAxcVC1b-.png?size=1600x1600"
  },
  "p16": {
    "plato": "Palito bombón x 6",
    "foto": "/images/platos/p16-palito-bombon-x-6.webp",
    "fuente": "https://commons.wikimedia.org/wiki/File:Liat_Portal_for_Foodie_Disorder_-_Chocolate_coated_ice_cream_bar_with_nuts.jpg",
    "autor": "HaJunkiyada",
    "licencia": "CC BY-SA 4.0"
  },
  "p17": {
    "plato": "Johnny classic burger",
    "foto": "/images/platos/p15-johnny-burger.webp",
    "fuente": "https://glovoapp.com/es/es/valencia/stores/bill-burger-valencia",
    "imagenOriginal": "https://images.deliveryhero.io/image/global-menu-service/GV_ES/vendor/915556/product/42213327296/41c947ba-019c-41a3-897f-94b3923523a3.jpg"
  },
  "p18": {
    "plato": "Costillas BBQ",
    "foto": "/images/platos/p18-costillas-bbq.webp",
    "fuente": "https://commons.wikimedia.org/wiki/File:Pork-ribs_02.jpg",
    "autor": "Pannet",
    "licencia": "CC BY 4.0"
  },
  "p19": {
    "plato": "Nachos con cheddar",
    "foto": "/images/platos/p19-nachos-con-cheddar.webp",
    "fuente": "https://commons.wikimedia.org/wiki/File:Nachos-cheese_(cropped).jpg",
    "autor": "chee.hong from Kuala Lumpur, Malaysia",
    "licencia": "CC BY 2.0"
  },
  "p21": {
    "plato": "Sorrentinos de jamón y queso",
    "foto": "/images/platos/p19-sorrentinos-con-crema-y-hongos.webp",
    "fuente": "https://img-global.cpcdn.com/steps/e8e6cd4dda718949/400x400cq80/photo.jpg",
    "imagenOriginal": "https://img-global.cpcdn.com/steps/e8e6cd4dda718949/400x400cq80/photo.jpg"
  },
  "p22": {
    "plato": "Ñoquis de papa",
    "foto": "/images/platos/p20-aoquis-pomodoro.webp",
    "fuente": "https://www.fabiospasta.com/",
    "imagenOriginal": "https://images.squarespace-cdn.com/content/v1/6003a6a76f0b7e66aaa2bc95/27d54864-b998-46ec-83a1-65bb14fab15b/IMG_4948-compressed.JPG?format=1500w"
  },
  "p23": {
    "plato": "Lasaña de verduras",
    "foto": "/images/platos/p23-lasana-de-verduras.webp",
    "fuente": "https://commons.wikimedia.org/wiki/File:Eggplant_Lasagna_(Vegitarian)_(1_of_6).jpg",
    "autor": "Wheeler Cowperthwaite",
    "licencia": "CC BY 2.0"
  },
  "p24": {
    "plato": "Tiramisú",
    "foto": "/images/platos/p24-tiramisu.webp",
    "fuente": "https://commons.wikimedia.org/wiki/File:Classic_Italian_Tiramisu-3_(29989504485).jpg",
    "autor": "Sharon Chen from Austin, United States",
    "licencia": "CC BY 2.0"
  },
  "p25": {
    "plato": "Bowl de quinoa",
    "foto": "/images/platos/p21-bowl-de-quinoa-y-pollo.webp",
    "fuente": "https://dynamic-media-cdn.tripadvisor.com/media/photo-o/0e/cb/41/8f/quinoa-salad-chicken.jpg?h=500&s=1&w=900",
    "imagenOriginal": "https://dynamic-media-cdn.tripadvisor.com/media/photo-o/0e/cb/41/8f/quinoa-salad-chicken.jpg?h=500&s=1&w=900"
  },
  "p26": {
    "plato": "Ensalada César con pollo",
    "foto": "/images/platos/p26-ensalada-cesar-con-pollo.webp",
    "fuente": "https://commons.wikimedia.org/wiki/File:Chicken_Caesar_salad_-_Auckland,_New_Zealand.jpg",
    "autor": "Daderot",
    "licencia": "CC0"
  },
  "p27": {
    "plato": "Wrap de falafel",
    "foto": "/images/platos/p27-wrap-de-falafel.webp",
    "fuente": "https://commons.wikimedia.org/wiki/File:Falafel_%26Tzatziki_Wrap_-_Lavash.jpg",
    "autor": "Andy Li",
    "licencia": "CC0"
  },
  "p28": {
    "plato": "Jugo verde",
    "foto": "/images/platos/p28-jugo-verde.webp",
    "fuente": "https://commons.wikimedia.org/wiki/File:Fresh_fruit_smoothie_preparation_with_assorted_fruits_on_a_table.jpg",
    "autor": "Shixart1985",
    "licencia": "CC BY 2.0"
  },
  "p29": {
    "plato": "Alfajores de chocolate x 6",
    "foto": "/images/platos/p26-alfajores-x-2.webp",
    "fuente": "https://havanna.com.ar/productos/alfajores",
    "imagenOriginal": "https://havanna.com.ar/data/upload/producto_galeria/b63a72771092539903b4c06aadaa05e9.png"
  },
  "p30": {
    "plato": "Café con leche y medialunas",
    "foto": "/images/platos/p24-cafa-con-leche-y-medialunas.webp",
    "fuente": "https://guiaoleo.com/buenos-aires/havanna-1",
    "imagenOriginal": "https://media.guiaoleo.com/cdn-cgi/image/width%3D600%2Cheight%3D450%2Cquality%3D80%2Cfit%3Dcover%2Cformat%3Dauto/restaurants/gallery/gallery_703_1771728130_699a6d0223488.jpg"
  },
  "p31": {
    "plato": "Havannets x 6",
    "foto": "/images/platos/p31-havannets-x-6.webp",
    "fuente": "Aportada por el equipo (foto de internet, ilustrativa)"
  },
  "p32": {
    "plato": "Torta de ricota",
    "foto": "/images/platos/p32-torta-de-ricota.webp",
    "fuente": "Aportada por el equipo (foto de internet, ilustrativa)"
  },
  "p33": {
    "plato": "Big Pons doble",
    "foto": "/images/platos/p29-pons-doble-bacon.webp",
    "fuente": "https://www.fondodeolla.com/files/image/54/54603/68c993e7ad8de_798_448%21.jpg?d=1758041972&s=baf606215360b224faf534ef19b101af",
    "imagenOriginal": "https://www.fondodeolla.com/files/image/54/54603/68c993e7ad8de_798_448%21.jpg?d=1758041972&s=baf606215360b224faf534ef19b101af"
  },
  "p34": {
    "plato": "Burger de pollo crispy",
    "foto": "/images/platos/p34-burger-de-pollo-crispy.webp",
    "fuente": "https://commons.wikimedia.org/wiki/File:Argentine_chicken_burger_sandwich_with_lettuce,_tomato,_fried_egg,_and_mozzarella_cheese_on_Don_Yeyo_bun_-_Front_view.jpg",
    "autor": "Horacio Cambeiro",
    "licencia": "CC BY-SA 3.0"
  },
  "p35": {
    "plato": "Papas con cheddar y panceta",
    "foto": "/images/platos/p41-papas-cheddar-y-bacon.webp",
    "fuente": "https://www.wendys.com.ar/pedir/KavuzGdEfEyXAf3s3/bacon-cheese-fries",
    "imagenOriginal": "https://tofuu.getjusto.com/orioneat-local/resized2/7vy7GFRDqgXPrpHEH-744-x.webp"
  },
  "p36": {
    "plato": "Aros de cebolla",
    "foto": "/images/platos/p36-aros-de-cebolla.webp",
    "fuente": "https://commons.wikimedia.org/wiki/File:Beer_Battered_Onion_Rings_-_RV90_-_20241107-133409.jpg",
    "autor": "RegionVisitor90",
    "licencia": "CC0"
  },
  "p37": {
    "plato": "Balde de pollo x 8",
    "foto": "/images/platos/p31-balde-de-pollo-x-8.webp",
    "fuente": "https://storage.pathaofood.live/food/b9d416cc-40b4-48ec-b948-9d936f6cb821.webp?ts=1691126429",
    "imagenOriginal": "https://storage.pathaofood.live/food/b9d416cc-40b4-48ec-b948-9d936f6cb821.webp?ts=1691126429"
  },
  "p38": {
    "plato": "Tiras de pollo picantes x 6",
    "foto": "/images/platos/p33-tiras-de-pollo-con-papas.webp",
    "fuente": "https://images.deliveryhero.io/image/hungerstation/menuitem/image_url_ref/3222eac25acf6b40a41c8526ec91a665.jpg?quality=75&width=828",
    "imagenOriginal": "https://images.deliveryhero.io/image/hungerstation/menuitem/image_url_ref/3222eac25acf6b40a41c8526ec91a665.jpg?quality=75&width=828"
  },
  "p39": {
    "plato": "Twister",
    "foto": "/images/platos/p39-twister.webp",
    "fuente": "https://commons.wikimedia.org/wiki/File:Smoked_chicken_and_avocado_wrap.jpg",
    "autor": "Takeaway",
    "licencia": "CC BY-SA 3.0"
  },
  "p40": {
    "plato": "Ensalada de repollo",
    "foto": "/images/platos/p40-ensalada-de-repollo.webp",
    "fuente": "https://commons.wikimedia.org/wiki/File:Bowl%27o%27Coleslaw_modified.jpg",
    "autor": "",
    "licencia": "CC BY-SA 2.0"
  },
  "p41": {
    "plato": "Milanesa napolitana",
    "foto": "/images/platos/p34-napolitana-con-papas.webp",
    "fuente": "https://www.canal26.com/resizer/v2/ULENIKHXDJANJFNF3VXFDFERGE.jpeg?auth=c110e88197ae7daf474718402b4ba864a6999a24f4fa17eff10a22350ff83a3e&height=630&quality=70&smart=true&width=1200",
    "imagenOriginal": "https://www.canal26.com/resizer/v2/ULENIKHXDJANJFNF3VXFDFERGE.jpeg?auth=c110e88197ae7daf474718402b4ba864a6999a24f4fa17eff10a22350ff83a3e&height=630&quality=70&smart=true&width=1200"
  },
  "p42": {
    "plato": "Milanesa a caballo",
    "foto": "/images/platos/p35-milanesa-de-pollo-a-caballo.webp",
    "fuente": "https://www.tripadvisor.de/Restaurant_Review-g948463-d12585750-Reviews-Los_Cipreses_de_Algarrobo-Algarrobo_Valparaiso_Region.html",
    "imagenOriginal": "https://media-cdn.tripadvisor.com/media/photo-s/13/2d/33/0c/img-20170919-140411-1.jpg"
  },
  "p43": {
    "plato": "Milanesa de pollo",
    "foto": "/images/platos/p43-milanesa-de-pollo.webp",
    "fuente": "https://commons.wikimedia.org/wiki/File:Chicken_milanesa_of_Argentina.jpg",
    "autor": "Federico L.",
    "licencia": "CC BY-SA 3.0"
  },
  "p44": {
    "plato": "Milanesa de berenjena",
    "foto": "/images/platos/p44-milanesa-de-berenjena.webp",
    "fuente": "Aportada por el equipo (foto de internet, ilustrativa)"
  },
  "p46": {
    "plato": "Dona de chocolate",
    "foto": "/images/platos/p36-donas-glaseadas-x-3.webp",
    "fuente": "https://news.dunkindonuts.com/file?fid=56bc83deacae6e3eab4f75c7",
    "imagenOriginal": "https://iprsoftwaremedia.com/285/files/201602/56bc83deacae6e3eab4f75c7_Chocolate_Frosted_Donut/Chocolate_Frosted_Donut_mid.jpg"
  },
  "p48": {
    "plato": "Munchkins x 12",
    "foto": "/images/platos/p48-munchkins-x-12.webp",
    "fuente": "Aportada por el equipo (foto de internet, ilustrativa)"
  },
  "p49": {
    "plato": "Dean doble cheddar",
    "foto": "/images/platos/p39-dean-doble-cheddar.webp",
    "fuente": "https://dean.ar/",
    "imagenOriginal": "https://dean.ar/imagenes/smashed/cheese%20smash.webp"
  },
  "p50": {
    "plato": "Triple bacon",
    "foto": "/images/platos/p40-bacon-cheeseburger.webp",
    "fuente": "https://dean.ar/",
    "imagenOriginal": "https://dean.ar/imagenes/clasicas/bacon%20cheese.webp"
  },
  "p51": {
    "plato": "Papas Dean",
    "foto": "/images/platos/p4-papas-con-cheddar.webp",
    "fuente": "https://tofuu.getjusto.com/orioneat-local/resized2/5LEz9MWWGWjCpfZec-2400-x.webp",
    "imagenOriginal": "https://tofuu.getjusto.com/orioneat-local/resized2/5LEz9MWWGWjCpfZec-2400-x.webp"
  },
  "p52": {
    "plato": "Milkshake de vainilla",
    "foto": "/images/platos/p52-milkshake-de-vainilla.webp",
    "fuente": "https://commons.wikimedia.org/wiki/File:Vanilla_milkshake_at_Lunchbox_Laboratories.jpg",
    "autor": "jencu from bay area, usa",
    "licencia": "CC BY 2.0"
  },
  "p53": {
    "plato": "Combinado Pop x 15",
    "foto": "/images/platos/p42-combinado-x-15.webp",
    "fuente": "https://www.sushipop.com.ar/menu",
    "imagenOriginal": "https://sushipopimg.s3.amazonaws.com/productos/carousel/WeuJiLn7RehtfkMNb95b7-2w_S0nqyk5.webp"
  },
  "p54": {
    "plato": "Hot roll x 10",
    "foto": "/images/platos/p8-tempura-de-langostinos-x-8.webp",
    "fuente": "https://d1w7312wesee68.cloudfront.net/ejuIbV3wHYF6llP0WPPq2JR58tMqF0015llIvRt0ZDw/resize%3Afit%3A720%3A720/plain/s3%3A/toasttab/restaurants/restaurant-205036000000000000/menu/items/1/item-1200000011310877221_1769096335.png",
    "imagenOriginal": "https://d1w7312wesee68.cloudfront.net/ejuIbV3wHYF6llP0WPPq2JR58tMqF0015llIvRt0ZDw/resize%3Afit%3A720%3A720/plain/s3%3A/toasttab/restaurants/restaurant-205036000000000000/menu/items/1/item-1200000011310877221_1769096335.png"
  },
  "p55": {
    "plato": "Ceviche de pescado blanco",
    "foto": "/images/platos/p55-ceviche-de-pescado-blanco.webp",
    "fuente": "https://commons.wikimedia.org/wiki/File:Ceviche_del_Per%C3%BA.jpg",
    "autor": "No machine-readable author provided. Manuel González Olaechea assumed (based on copyright claims).",
    "licencia": "CC BY 3.0"
  },
  "p56": {
    "plato": "Wok de vegetales",
    "foto": "/images/platos/p56-wok-de-vegetales.webp",
    "fuente": "https://commons.wikimedia.org/wiki/File:Stir_fried_mixed_vegetables.jpg",
    "autor": "CNEcija12345",
    "licencia": "CC BY-SA 4.0"
  },
  "p57": {
    "plato": "Ojo de bife",
    "foto": "/images/platos/p46-vacao-con-ensalada.webp",
    "fuente": "https://club.lanacion.com.ar/beneficios/gastronomia/restaurantes/descuentos-en-la-cabrera-A01593993",
    "imagenOriginal": "https://club.lanacion.com.ar/lnestablecimientos/imagenes/A01593993/B775962.jpg"
  },
  "p58": {
    "plato": "Bife de chorizo",
    "foto": "/images/platos/p45-bife-de-chorizo-con-papas.webp",
    "fuente": "https://tn.com.ar/tecno/redes-sociales/2024/07/12/es-brasilena-fue-a-comer-a-un-reconocido-restaurante-de-palermo-y-mostro-el-ticket-final/",
    "imagenOriginal": "https://tn.com.ar/resizer/v2/la-mujer-pidio-bife-de-chorizo-con-papas-fritas-foto-googlemapsmariana-pagano-GUYYR5JQFVE27HM4O32A22LR7A.PNG?auth=6ffa2300a87fa6cae2add38d6f3472dfd2b593ea49263ec610c7fad6e329d258&width=767"
  },
  "p59": {
    "plato": "Provoleta",
    "foto": "/images/platos/p47-provoleta.webp",
    "fuente": "https://www.tasteatlas.com/lacabrera/grilledprovoleta",
    "imagenOriginal": "https://cdn.tasteatlas.com/images/dishrestaurants/5d57c98352484bbdb71efbd17315f1fd.jpg?height=513&width=800"
  },
  "p60": {
    "plato": "Flan casero",
    "foto": "/images/platos/p60-flan-casero.webp",
    "fuente": "https://commons.wikimedia.org/wiki/File:Cr%C3%A8me_caramel_2.jpg",
    "autor": "Amanda  Coutinho",
    "licencia": "CC BY 2.0"
  },
  "p61": {
    "plato": "Tostado de jamón y queso",
    "foto": "/images/platos/p61-tostado-de-jamon-y-queso.webp",
    "fuente": "https://commons.wikimedia.org/wiki/File:Ham_%26_cheese_toasted_sandwich_-_Potatoast_2026-05-02.jpg",
    "autor": "Andy Li",
    "licencia": "CC0"
  },
  "p62": {
    "plato": "Café en grano 250 g",
    "foto": "/images/platos/p62-cafe-en-grano-250-g.webp",
    "fuente": "https://commons.wikimedia.org/wiki/File:Roasted_coffee_beans.jpg",
    "autor": "MarkSweep",
    "licencia": "Public domain"
  },
  "p63": {
    "plato": "Cheesecake de frutos rojos",
    "foto": "/images/platos/p63-cheesecake-de-frutos-rojos.webp",
    "fuente": "https://commons.wikimedia.org/wiki/File:Strawberry_cheesecake_round.jpg",
    "autor": "Josefine Linnea Jonsson",
    "licencia": "CC BY-SA 4.0"
  },
  "p64": {
    "plato": "Licuado de banana",
    "foto": "/images/platos/p64-licuado-de-banana.webp",
    "fuente": "https://commons.wikimedia.org/wiki/File:Banana_Smoothie.jpg",
    "autor": "Ourlibrary",
    "licencia": "CC0"
  }
};
