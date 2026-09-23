# Punto 1 — Network Monitor: monitoreo de conexiones e IPs

Agregar a Go-Data un nuevo módulo llamado **Network Monitor** para monitorear las conexiones de red que recibe el servidor.

El objetivo NO es monitorear Minecraft, Crafty ni jugadores específicamente. El objetivo es obtener información sobre **las conexiones de red del servidor en general**, independientemente del servicio que las atienda.

La arquitectura conceptual es:

```text
INTERNET
    │
    ▼
┌──────────────────────────┐
│        SERVIDOR          │
│                          │
│        Go-Data           │
│           │              │
│   ┌───────▼────────┐     │
│   │ Network Monitor │     │
│   └───────┬────────┘     │
│           │              │
└───────────┼──────────────┘
            │
   ┌────────┼────────────┐
   ▼        ▼            ▼
  TCP      UDP          HTTP
 :25565   :xxxxx       :80/:443
 :8085    :xxxxx       :8080
 :9000
```

## Información a obtener

El Network Monitor debe poder detectar y registrar, como mínimo:

* IP remota.
* Puerto remoto.
* Puerto local.
* Protocolo: TCP / UDP.
* Dirección del tráfico: inbound / outbound.
* Estado de la conexión cuando corresponda.
* Timestamp.
* Primera vez que se observó una IP.
* Última vez que se observó una IP.

Por ejemplo:

```text
181.xxx.xxx.xxx → TCP → :25565
190.xxx.xxx.xxx → TCP → :8085
200.xxx.xxx.xxx → TCP → :9000
201.xxx.xxx.xxx → UDP → :25565
```

## Información agregada

El sistema debería permitir obtener:

* Conexiones activas actualmente.
* Cantidad de IPs únicas.
* Cantidad de conexiones por IP.
* Cantidad de conexiones por puerto.
* Cantidad de conexiones por protocolo.
* TCP activo.
* UDP activo.
* Historial de IPs detectadas.
* Primera vez que una IP fue detectada.
* Última vez que una IP fue detectada.

También debería ser posible consultar algo como:

```text
IP                    PROTO    PUERTO    DIRECCIÓN    ESTADO
181.xxx.xxx.xxx       TCP      25565     INBOUND      ESTABLISHED
190.xxx.xxx.xxx       TCP      8085      INBOUND      ESTABLISHED
200.xxx.xxx.xxx       TCP      9000      INBOUND      ESTABLISHED
201.xxx.xxx.xxx       UDP      25565     INBOUND      ACTIVE
```

## Persistencia

Integrar el módulo con la arquitectura actual de Go-Data y utilizar **InfluxDB** cuando corresponda.

Evitar almacenar cada detalle de cada conexión indefinidamente si eso genera un volumen innecesario de datos.

Separar correctamente:

* métricas;
* estado actual;
* eventos/historial.

El collector debe ser modular y estar desacoplado del resto de los collectors existentes.

## Dashboard

Agregar una sección de Network Monitor al dashboard de Go-Data.

Como mínimo debería mostrar:

* Conexiones activas.
* IPs únicas.
* TCP.
* UDP.
* Principales IPs.
* Principales puertos.
* Últimas conexiones detectadas.
* Historial de IPs.
* Primera vez vista.
* Última vez vista.

Ejemplo conceptual:

```text
NETWORK MONITOR

Active connections       12
Unique IPs                8
TCP connections           10
UDP activity               2


TOP IPs

181.xxx.xxx.xxx            4 connections
190.xxx.xxx.xxx            2 connections
200.xxx.xxx.xxx            1 connection


PORTS

25565 TCP                  4
8085  TCP                  2
9000  TCP                  1
443   TCP                  3
25565 UDP                  2


RECENT CONNECTIONS

17:31   181.xxx.xxx.xxx   TCP   :25565   INBOUND
17:30   190.xxx.xxx.xxx   TCP   :8085    INBOUND
17:29   201.xxx.xxx.xxx   UDP   :25565   INBOUND
```

## Implementación

Investigar el mecanismo más adecuado en Linux para obtener esta información con bajo consumo de recursos.

Para TCP se puede utilizar información disponible mediante `/proc`, `ss`, netlink u otra alternativa adecuada.

Para UDP tener en cuenta que no existe el mismo concepto de conexión persistente que en TCP. Utilizar el mecanismo apropiado para detectar actividad UDP.

Evitar realizar una captura permanente de paquetes si no es necesaria.

La solución debe priorizar:

1. Bajo consumo de CPU.
2. Bajo consumo de RAM.
3. Bajo impacto en el servidor.
4. Arquitectura modular.
5. No depender de Crafty.
6. No depender de Minecraft.
7. Poder extenderse posteriormente a otros tipos de tráfico.

## Importante

No interesa identificar jugadores de Minecraft.

No interesa saber qué usuario de Minecraft está conectado.

Solo interesa saber:

**qué IPs se comunican con el servidor, mediante qué protocolo, hacia qué puerto y cuándo.**

El módulo debe funcionar para el servidor completo y no para una aplicación específica.
