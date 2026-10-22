"""Envíos: zona de destino por código postal, tarifas por peso, recargos y plazos."""

# Zona de cada prefijo de código postal (tres primeras cifras). Las zonas: 1 centro, 2 capitales peninsulares,
# 3 resto de la península, 4 Baleares, 5 Canarias, Ceuta y Melilla.
ZONAS = {
    "010": 3,  # Álava
    "011": 3,  # Álava
    "012": 3,  # Álava
    "013": 3,  # Álava
    "014": 3,  # Álava
    "015": 3,  # Álava
    "016": 3,  # Álava
    "017": 3,  # Álava
    "018": 3,  # Álava
    "019": 3,  # Álava
    "020": 3,  # Albacete
    "021": 3,  # Albacete
    "022": 3,  # Albacete
    "023": 3,  # Albacete
    "024": 3,  # Albacete
    "025": 3,  # Albacete
    "026": 3,  # Albacete
    "027": 3,  # Albacete
    "028": 3,  # Albacete
    "029": 3,  # Albacete
    "030": 2,  # Alicante
    "031": 2,  # Alicante
    "032": 2,  # Alicante
    "033": 2,  # Alicante
    "034": 2,  # Alicante
    "035": 2,  # Alicante
    "036": 2,  # Alicante
    "037": 2,  # Alicante
    "038": 2,  # Alicante
    "039": 2,  # Alicante
    "040": 3,  # Almería
    "041": 3,  # Almería
    "042": 3,  # Almería
    "043": 3,  # Almería
    "044": 3,  # Almería
    "045": 3,  # Almería
    "046": 3,  # Almería
    "047": 3,  # Almería
    "048": 3,  # Almería
    "049": 3,  # Almería
    "050": 1,  # Ávila
    "051": 1,  # Ávila
    "052": 1,  # Ávila
    "053": 1,  # Ávila
    "054": 1,  # Ávila
    "055": 1,  # Ávila
    "056": 1,  # Ávila
    "057": 1,  # Ávila
    "058": 1,  # Ávila
    "059": 1,  # Ávila
    "060": 3,  # Badajoz
    "061": 3,  # Badajoz
    "062": 3,  # Badajoz
    "063": 3,  # Badajoz
    "064": 3,  # Badajoz
    "065": 3,  # Badajoz
    "066": 3,  # Badajoz
    "067": 3,  # Badajoz
    "068": 3,  # Badajoz
    "069": 3,  # Badajoz
    "070": 4,  # Baleares
    "071": 4,  # Baleares
    "072": 4,  # Baleares
    "073": 4,  # Baleares
    "074": 4,  # Baleares
    "075": 4,  # Baleares
    "076": 4,  # Baleares
    "077": 4,  # Baleares
    "078": 4,  # Baleares
    "079": 4,  # Baleares
    "080": 2,  # Barcelona
    "081": 2,  # Barcelona
    "082": 2,  # Barcelona
    "083": 2,  # Barcelona
    "084": 2,  # Barcelona
    "085": 2,  # Barcelona
    "086": 2,  # Barcelona
    "087": 2,  # Barcelona
    "088": 2,  # Barcelona
    "089": 2,  # Barcelona
    "090": 3,  # Burgos
    "091": 3,  # Burgos
    "092": 3,  # Burgos
    "093": 3,  # Burgos
    "094": 3,  # Burgos
    "095": 3,  # Burgos
    "096": 3,  # Burgos
    "097": 3,  # Burgos
    "098": 3,  # Burgos
    "099": 3,  # Burgos
    "100": 3,  # Cáceres
    "101": 3,  # Cáceres
    "102": 3,  # Cáceres
    "103": 3,  # Cáceres
    "104": 3,  # Cáceres
    "105": 3,  # Cáceres
    "106": 3,  # Cáceres
    "107": 3,  # Cáceres
    "108": 3,  # Cáceres
    "109": 3,  # Cáceres
    "110": 3,  # Cádiz
    "111": 3,  # Cádiz
    "112": 3,  # Cádiz
    "113": 3,  # Cádiz
    "114": 3,  # Cádiz
    "115": 3,  # Cádiz
    "116": 3,  # Cádiz
    "117": 3,  # Cádiz
    "118": 3,  # Cádiz
    "119": 3,  # Cádiz
    "120": 3,  # Castellón
    "121": 3,  # Castellón
    "122": 3,  # Castellón
    "123": 3,  # Castellón
    "124": 3,  # Castellón
    "125": 3,  # Castellón
    "126": 3,  # Castellón
    "127": 3,  # Castellón
    "128": 3,  # Castellón
    "129": 3,  # Castellón
    "130": 3,  # Ciudad Real
    "131": 3,  # Ciudad Real
    "132": 3,  # Ciudad Real
    "133": 3,  # Ciudad Real
    "134": 3,  # Ciudad Real
    "135": 3,  # Ciudad Real
    "136": 3,  # Ciudad Real
    "137": 3,  # Ciudad Real
    "138": 3,  # Ciudad Real
    "139": 3,  # Ciudad Real
    "140": 3,  # Córdoba
    "141": 3,  # Córdoba
    "142": 3,  # Córdoba
    "143": 3,  # Córdoba
    "144": 3,  # Córdoba
    "145": 3,  # Córdoba
    "146": 3,  # Córdoba
    "147": 3,  # Córdoba
    "148": 3,  # Córdoba
    "149": 3,  # Córdoba
    "150": 3,  # A Coruña
    "151": 3,  # A Coruña
    "152": 3,  # A Coruña
    "153": 3,  # A Coruña
    "154": 3,  # A Coruña
    "155": 3,  # A Coruña
    "156": 3,  # A Coruña
    "157": 3,  # A Coruña
    "158": 3,  # A Coruña
    "159": 3,  # A Coruña
    "160": 1,  # Cuenca
    "161": 1,  # Cuenca
    "162": 1,  # Cuenca
    "163": 1,  # Cuenca
    "164": 1,  # Cuenca
    "165": 1,  # Cuenca
    "166": 1,  # Cuenca
    "167": 1,  # Cuenca
    "168": 1,  # Cuenca
    "169": 1,  # Cuenca
    "170": 3,  # Girona
    "171": 3,  # Girona
    "172": 3,  # Girona
    "173": 3,  # Girona
    "174": 3,  # Girona
    "175": 3,  # Girona
    "176": 3,  # Girona
    "177": 3,  # Girona
    "178": 3,  # Girona
    "179": 3,  # Girona
    "180": 3,  # Granada
    "181": 3,  # Granada
    "182": 3,  # Granada
    "183": 3,  # Granada
    "184": 3,  # Granada
    "185": 3,  # Granada
    "186": 3,  # Granada
    "187": 3,  # Granada
    "188": 3,  # Granada
    "189": 3,  # Granada
    "190": 1,  # Guadalajara
    "191": 1,  # Guadalajara
    "192": 1,  # Guadalajara
    "193": 1,  # Guadalajara
    "194": 1,  # Guadalajara
    "195": 1,  # Guadalajara
    "196": 1,  # Guadalajara
    "197": 1,  # Guadalajara
    "198": 1,  # Guadalajara
    "199": 1,  # Guadalajara
    "200": 3,  # Gipuzkoa
    "201": 3,  # Gipuzkoa
    "202": 3,  # Gipuzkoa
    "203": 3,  # Gipuzkoa
    "204": 3,  # Gipuzkoa
    "205": 3,  # Gipuzkoa
    "206": 3,  # Gipuzkoa
    "207": 3,  # Gipuzkoa
    "208": 3,  # Gipuzkoa
    "209": 3,  # Gipuzkoa
    "210": 3,  # Huelva
    "211": 3,  # Huelva
    "212": 3,  # Huelva
    "213": 3,  # Huelva
    "214": 3,  # Huelva
    "215": 3,  # Huelva
    "216": 3,  # Huelva
    "217": 3,  # Huelva
    "218": 3,  # Huelva
    "219": 3,  # Huelva
    "220": 3,  # Huesca
    "221": 3,  # Huesca
    "222": 3,  # Huesca
    "223": 3,  # Huesca
    "224": 3,  # Huesca
    "225": 3,  # Huesca
    "226": 3,  # Huesca
    "227": 3,  # Huesca
    "228": 3,  # Huesca
    "229": 3,  # Huesca
    "230": 3,  # Jaén
    "231": 3,  # Jaén
    "232": 3,  # Jaén
    "233": 3,  # Jaén
    "234": 3,  # Jaén
    "235": 3,  # Jaén
    "236": 3,  # Jaén
    "237": 3,  # Jaén
    "238": 3,  # Jaén
    "239": 3,  # Jaén
    "240": 3,  # León
    "241": 3,  # León
    "242": 3,  # León
    "243": 3,  # León
    "244": 3,  # León
    "245": 3,  # León
    "246": 3,  # León
    "247": 3,  # León
    "248": 3,  # León
    "249": 3,  # León
    "250": 3,  # Lleida
    "251": 3,  # Lleida
    "252": 3,  # Lleida
    "253": 3,  # Lleida
    "254": 3,  # Lleida
    "255": 3,  # Lleida
    "256": 3,  # Lleida
    "257": 3,  # Lleida
    "258": 3,  # Lleida
    "259": 3,  # Lleida
    "260": 3,  # La Rioja
    "261": 3,  # La Rioja
    "262": 3,  # La Rioja
    "263": 3,  # La Rioja
    "264": 3,  # La Rioja
    "265": 3,  # La Rioja
    "266": 3,  # La Rioja
    "267": 3,  # La Rioja
    "268": 3,  # La Rioja
    "269": 3,  # La Rioja
    "270": 3,  # Lugo
    "271": 3,  # Lugo
    "272": 3,  # Lugo
    "273": 3,  # Lugo
    "274": 3,  # Lugo
    "275": 3,  # Lugo
    "276": 3,  # Lugo
    "277": 3,  # Lugo
    "278": 3,  # Lugo
    "279": 3,  # Lugo
    "280": 1,  # Madrid
    "281": 1,  # Madrid
    "282": 1,  # Madrid
    "283": 1,  # Madrid
    "284": 1,  # Madrid
    "285": 1,  # Madrid
    "286": 1,  # Madrid
    "287": 1,  # Madrid
    "288": 1,  # Madrid
    "289": 1,  # Madrid
    "290": 2,  # Málaga
    "291": 2,  # Málaga
    "292": 2,  # Málaga
    "293": 2,  # Málaga
    "294": 2,  # Málaga
    "295": 2,  # Málaga
    "296": 2,  # Málaga
    "297": 2,  # Málaga
    "298": 2,  # Málaga
    "299": 2,  # Málaga
    "300": 2,  # Murcia
    "301": 2,  # Murcia
    "302": 2,  # Murcia
    "303": 2,  # Murcia
    "304": 2,  # Murcia
    "305": 2,  # Murcia
    "306": 2,  # Murcia
    "307": 2,  # Murcia
    "308": 2,  # Murcia
    "309": 2,  # Murcia
    "310": 3,  # Navarra
    "311": 3,  # Navarra
    "312": 3,  # Navarra
    "313": 3,  # Navarra
    "314": 3,  # Navarra
    "315": 3,  # Navarra
    "316": 3,  # Navarra
    "317": 3,  # Navarra
    "318": 3,  # Navarra
    "319": 3,  # Navarra
    "320": 3,  # Ourense
    "321": 3,  # Ourense
    "322": 3,  # Ourense
    "323": 3,  # Ourense
    "324": 3,  # Ourense
    "325": 3,  # Ourense
    "326": 3,  # Ourense
    "327": 3,  # Ourense
    "328": 3,  # Ourense
    "329": 3,  # Ourense
    "330": 3,  # Asturias
    "331": 3,  # Asturias
    "332": 3,  # Asturias
    "333": 3,  # Asturias
    "334": 3,  # Asturias
    "335": 3,  # Asturias
    "336": 3,  # Asturias
    "337": 3,  # Asturias
    "338": 3,  # Asturias
    "339": 3,  # Asturias
    "340": 3,  # Palencia
    "341": 3,  # Palencia
    "342": 3,  # Palencia
    "343": 3,  # Palencia
    "344": 3,  # Palencia
    "345": 3,  # Palencia
    "346": 3,  # Palencia
    "347": 3,  # Palencia
    "348": 3,  # Palencia
    "349": 3,  # Palencia
    "350": 5,  # Las Palmas
    "351": 5,  # Las Palmas
    "352": 5,  # Las Palmas
    "353": 5,  # Las Palmas
    "354": 5,  # Las Palmas
    "355": 5,  # Las Palmas
    "356": 5,  # Las Palmas
    "357": 5,  # Las Palmas
    "358": 5,  # Las Palmas
    "359": 5,  # Las Palmas
    "360": 3,  # Pontevedra
    "361": 3,  # Pontevedra
    "362": 3,  # Pontevedra
    "363": 3,  # Pontevedra
    "364": 3,  # Pontevedra
    "365": 3,  # Pontevedra
    "366": 3,  # Pontevedra
    "367": 3,  # Pontevedra
    "368": 3,  # Pontevedra
    "369": 3,  # Pontevedra
    "370": 3,  # Salamanca
    "371": 3,  # Salamanca
    "372": 3,  # Salamanca
    "373": 3,  # Salamanca
    "374": 3,  # Salamanca
    "375": 3,  # Salamanca
    "376": 3,  # Salamanca
    "377": 3,  # Salamanca
    "378": 3,  # Salamanca
    "379": 3,  # Salamanca
    "380": 5,  # Santa Cruz de Tenerife
    "381": 5,  # Santa Cruz de Tenerife
    "382": 5,  # Santa Cruz de Tenerife
    "383": 5,  # Santa Cruz de Tenerife
    "384": 5,  # Santa Cruz de Tenerife
    "385": 5,  # Santa Cruz de Tenerife
    "386": 5,  # Santa Cruz de Tenerife
    "387": 5,  # Santa Cruz de Tenerife
    "388": 5,  # Santa Cruz de Tenerife
    "389": 5,  # Santa Cruz de Tenerife
    "390": 3,  # Cantabria
    "391": 3,  # Cantabria
    "392": 3,  # Cantabria
    "393": 3,  # Cantabria
    "394": 3,  # Cantabria
    "395": 3,  # Cantabria
    "396": 3,  # Cantabria
    "397": 3,  # Cantabria
    "398": 3,  # Cantabria
    "399": 3,  # Cantabria
    "400": 1,  # Segovia
    "401": 1,  # Segovia
    "402": 1,  # Segovia
    "403": 1,  # Segovia
    "404": 1,  # Segovia
    "405": 1,  # Segovia
    "406": 1,  # Segovia
    "407": 1,  # Segovia
    "408": 1,  # Segovia
    "409": 1,  # Segovia
    "410": 2,  # Sevilla
    "411": 2,  # Sevilla
    "412": 2,  # Sevilla
    "413": 2,  # Sevilla
    "414": 2,  # Sevilla
    "415": 2,  # Sevilla
    "416": 2,  # Sevilla
    "417": 2,  # Sevilla
    "418": 2,  # Sevilla
    "419": 2,  # Sevilla
    "420": 3,  # Soria
    "421": 3,  # Soria
    "422": 3,  # Soria
    "423": 3,  # Soria
    "424": 3,  # Soria
    "425": 3,  # Soria
    "426": 3,  # Soria
    "427": 3,  # Soria
    "428": 3,  # Soria
    "429": 3,  # Soria
    "430": 3,  # Tarragona
    "431": 3,  # Tarragona
    "432": 3,  # Tarragona
    "433": 3,  # Tarragona
    "434": 3,  # Tarragona
    "435": 3,  # Tarragona
    "436": 3,  # Tarragona
    "437": 3,  # Tarragona
    "438": 3,  # Tarragona
    "439": 3,  # Tarragona
    "440": 3,  # Teruel
    "441": 3,  # Teruel
    "442": 3,  # Teruel
    "443": 3,  # Teruel
    "444": 3,  # Teruel
    "445": 3,  # Teruel
    "446": 3,  # Teruel
    "447": 3,  # Teruel
    "448": 3,  # Teruel
    "449": 3,  # Teruel
    "450": 1,  # Toledo
    "451": 1,  # Toledo
    "452": 1,  # Toledo
    "453": 1,  # Toledo
    "454": 1,  # Toledo
    "455": 1,  # Toledo
    "456": 1,  # Toledo
    "457": 1,  # Toledo
    "458": 1,  # Toledo
    "459": 1,  # Toledo
    "460": 2,  # Valencia
    "461": 2,  # Valencia
    "462": 2,  # Valencia
    "463": 2,  # Valencia
    "464": 2,  # Valencia
    "465": 2,  # Valencia
    "466": 2,  # Valencia
    "467": 2,  # Valencia
    "468": 2,  # Valencia
    "469": 2,  # Valencia
    "470": 2,  # Valladolid
    "471": 2,  # Valladolid
    "472": 2,  # Valladolid
    "473": 2,  # Valladolid
    "474": 2,  # Valladolid
    "475": 2,  # Valladolid
    "476": 2,  # Valladolid
    "477": 2,  # Valladolid
    "478": 2,  # Valladolid
    "479": 2,  # Valladolid
    "480": 2,  # Bizkaia
    "481": 2,  # Bizkaia
    "482": 2,  # Bizkaia
    "483": 2,  # Bizkaia
    "484": 2,  # Bizkaia
    "485": 2,  # Bizkaia
    "486": 2,  # Bizkaia
    "487": 2,  # Bizkaia
    "488": 2,  # Bizkaia
    "489": 2,  # Bizkaia
    "490": 3,  # Zamora
    "491": 3,  # Zamora
    "492": 3,  # Zamora
    "493": 3,  # Zamora
    "494": 3,  # Zamora
    "495": 3,  # Zamora
    "496": 3,  # Zamora
    "497": 3,  # Zamora
    "498": 3,  # Zamora
    "499": 3,  # Zamora
    "500": 2,  # Zaragoza
    "501": 2,  # Zaragoza
    "502": 2,  # Zaragoza
    "503": 2,  # Zaragoza
    "504": 2,  # Zaragoza
    "505": 2,  # Zaragoza
    "506": 2,  # Zaragoza
    "507": 2,  # Zaragoza
    "508": 2,  # Zaragoza
    "509": 2,  # Zaragoza
    "510": 5,  # Ceuta
    "511": 5,  # Ceuta
    "512": 5,  # Ceuta
    "513": 5,  # Ceuta
    "514": 5,  # Ceuta
    "515": 5,  # Ceuta
    "516": 5,  # Ceuta
    "517": 5,  # Ceuta
    "518": 5,  # Ceuta
    "519": 5,  # Ceuta
    "520": 5,  # Melilla
    "521": 5,  # Melilla
    "522": 5,  # Melilla
    "523": 5,  # Melilla
    "524": 5,  # Melilla
    "525": 5,  # Melilla
    "526": 5,  # Melilla
    "527": 5,  # Melilla
    "528": 5,  # Melilla
    "529": 5,  # Melilla
}

# Tarifa por zona: [(peso máximo en kg, precio)]. Por encima del último tramo, el último precio más el kilo extra.
TARIFAS = {
    1: [(1, 3.9), (2, 5.27), (5, 6.63), (10, 7.99), (15, 9.36), (20, 10.72), (30, 12.09), (40, 13.45)],
    2: [(1, 4.6), (2, 6.21), (5, 7.82), (10, 9.43), (15, 11.04), (20, 12.65), (30, 14.26), (40, 15.87)],
    3: [(1, 5.4), (2, 7.29), (5, 9.18), (10, 11.07), (15, 12.96), (20, 14.85), (30, 16.74), (40, 18.63)],
    4: [(1, 9.8), (2, 13.23), (5, 16.66), (10, 20.09), (15, 23.52), (20, 26.95), (30, 30.38), (40, 33.81)],
    5: [(1, 14.2), (2, 19.17), (5, 24.14), (10, 29.11), (15, 34.08), (20, 39.05), (30, 44.02), (40, 48.99)],
}

KILO_EXTRA = {1: 0.45, 2: 0.55, 3: 0.6, 4: 1.1, 5: 1.6}

# Recargo por envío urgente (24 h) por zona, en euros.
RECARGO_URGENTE = {1: 3.5, 2: 4.0, 3: 5.25, 4: 8.0, 5: 12.75}

# Plazo de entrega en días laborables por zona y almacén de salida.
PLAZOS = {
    "MAD": {1: 1, 2: 1, 3: 2, 4: 2, 5: 5},
    "BCN": {1: 1, 2: 2, 3: 5, 4: 5, 5: 3},
    "VLC": {1: 1, 2: 1, 3: 2, 4: 3, 5: 5},
    "SEV": {1: 2, 2: 1, 3: 4, 4: 3, 5: 4},
    "BIL": {1: 2, 2: 2, 3: 3, 4: 3, 5: 3},
}

# Festivos locales por provincia (día-mes) que retrasan las entregas.
FESTIVOS = {
    "01": ["02-06", "13-03", "21-05", "08-11", "18-12"],  # Álava
    "02": ["07-08", "05-12"],  # Albacete
    "03": ["14-06", "22-07", "04-01"],  # Alicante
    "04": ["04-11", "07-11", "17-09", "03-05"],  # Almería
    "05": ["12-01", "25-08", "03-04", "16-05", "28-05"],  # Ávila
    "06": ["07-03", "16-05"],  # Badajoz
    "07": ["19-05", "02-10", "20-02"],  # Baleares
    "08": ["12-04", "05-11"],  # Barcelona
    "09": ["02-03", "11-06", "15-08", "08-06"],  # Burgos
    "10": ["06-02", "26-05", "26-02", "24-09"],  # Cáceres
    "11": ["04-12", "18-02", "26-03", "20-07", "15-01"],  # Cádiz
    "12": ["02-09", "19-02"],  # Castellón
    "13": ["21-12", "05-07", "19-06", "03-06", "24-11"],  # Ciudad Real
    "14": ["12-03", "22-02", "11-01"],  # Córdoba
    "15": ["10-03", "09-02", "04-04", "04-03", "16-05"],  # A Coruña
    "16": ["11-08", "08-03"],  # Cuenca
    "17": ["17-05", "12-04"],  # Girona
    "18": ["13-09", "07-03", "08-12", "28-09"],  # Granada
    "19": ["04-01", "04-01", "16-12"],  # Guadalajara
    "20": ["23-12", "08-02", "25-03"],  # Gipuzkoa
    "21": ["27-05", "01-07", "13-10"],  # Huelva
    "22": ["10-10", "04-02"],  # Huesca
    "23": ["08-04", "20-09", "23-01"],  # Jaén
    "24": ["03-10", "11-02", "02-04"],  # León
    "25": ["27-05", "11-02", "26-08"],  # Lleida
    "26": ["01-06", "14-07", "02-02"],  # La Rioja
    "27": ["05-12", "17-11", "06-03"],  # Lugo
    "28": ["25-03", "07-04", "08-11", "11-12"],  # Madrid
    "29": ["01-08", "02-08"],  # Málaga
    "30": ["03-10", "21-02", "07-11", "02-06"],  # Murcia
    "31": ["03-11", "23-06", "19-03", "26-08", "22-12"],  # Navarra
    "32": ["05-05", "27-12", "10-01", "24-08", "27-11"],  # Ourense
    "33": ["14-07", "27-11", "26-09"],  # Asturias
    "34": ["24-10", "18-11", "21-02", "03-05"],  # Palencia
    "35": ["08-04", "19-08", "18-04"],  # Las Palmas
    "36": ["19-11", "23-01", "13-11", "26-07", "26-11"],  # Pontevedra
    "37": ["27-07", "13-02", "08-11", "22-06"],  # Salamanca
    "38": ["26-05", "01-05", "16-10", "01-02", "26-08"],  # Santa Cruz de Tenerife
    "39": ["14-10", "10-08", "05-06", "18-04", "03-06"],  # Cantabria
    "40": ["28-08", "20-01", "10-06", "03-05", "06-12"],  # Segovia
    "41": ["14-11", "18-04", "04-04", "22-11", "02-07"],  # Sevilla
    "42": ["13-05", "11-03", "12-03"],  # Soria
    "43": ["12-10", "13-05", "16-06"],  # Tarragona
    "44": ["28-03", "13-09", "01-01"],  # Teruel
    "45": ["04-04", "15-10", "26-11"],  # Toledo
    "46": ["24-06", "22-02", "18-12", "28-09"],  # Valencia
    "47": ["05-05", "22-07", "03-09", "20-06", "15-05"],  # Valladolid
    "48": ["12-05", "22-12", "21-11", "13-09"],  # Bizkaia
    "49": ["21-08", "16-06"],  # Zamora
    "50": ["02-11", "04-09"],  # Zaragoza
    "51": ["15-05", "25-09", "05-12", "20-12", "15-01"],  # Ceuta
    "52": ["16-03", "01-05", "05-04", "19-10"],  # Melilla
}


def zona_de(cp):
    """Zona de un código postal (5 cifras)."""
    try:
        return ZONAS[cp[:3]]
    except KeyError:
        raise ValueError(f"código postal desconocido: {cp}")


def coste_envio(peso, zona):
    """Coste del envío de un paquete de «peso» kg a una zona."""
    tramos = TARIFAS[zona]
    for maximo, precio in tramos:
        if peso <= maximo:
            return precio
    maximo, precio = tramos[-1]
    extra = peso - maximo
    return round(precio + extra * KILO_EXTRA[zona], 2)


def plazo(almacen, cp):
    return PLAZOS[almacen][zona_de(cp)]


def provincias_zona_1():
    """Prefijos de provincia de la zona 1."""
    return sorted({p[:2] for p, z in ZONAS.items() if z == 1})


def tabla_zona_1():
    """La tarifa de la zona 1 como texto, un tramo por línea."""
    return "\n".join(f"hasta {m} kg: {p:.2f} €" for m, p in TARIFAS[1])


def provincias_zona_2():
    """Prefijos de provincia de la zona 2."""
    return sorted({p[:2] for p, z in ZONAS.items() if z == 2})


def tabla_zona_2():
    """La tarifa de la zona 2 como texto, un tramo por línea."""
    return "\n".join(f"hasta {m} kg: {p:.2f} €" for m, p in TARIFAS[2])


def provincias_zona_3():
    """Prefijos de provincia de la zona 3."""
    return sorted({p[:2] for p, z in ZONAS.items() if z == 3})


def tabla_zona_3():
    """La tarifa de la zona 3 como texto, un tramo por línea."""
    return "\n".join(f"hasta {m} kg: {p:.2f} €" for m, p in TARIFAS[3])


def provincias_zona_4():
    """Prefijos de provincia de la zona 4."""
    return sorted({p[:2] for p, z in ZONAS.items() if z == 4})


def tabla_zona_4():
    """La tarifa de la zona 4 como texto, un tramo por línea."""
    return "\n".join(f"hasta {m} kg: {p:.2f} €" for m, p in TARIFAS[4])


def provincias_zona_5():
    """Prefijos de provincia de la zona 5."""
    return sorted({p[:2] for p, z in ZONAS.items() if z == 5})


def tabla_zona_5():
    """La tarifa de la zona 5 como texto, un tramo por línea."""
    return "\n".join(f"hasta {m} kg: {p:.2f} €" for m, p in TARIFAS[5])

