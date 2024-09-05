# TP0: Docker + Comunicaciones + Concurrencia

## Índice

1. [Introducción a Docker](#parte-1-introducción-a-docker)
   - [Ejercicio N°1](#ejercicio-n1)
   - [Ejercicio N°2](#ejercicio-n2)
   - [Ejercicio N°3](#ejercicio-n3)
   - [Ejercicio N°4](#ejercicio-n4)
2. [Repaso de Comunicaciones](#parte-2-repaso-de-comunicaciones)
   - [Ejercicio N°5](#ejercicio-n5)
   - [Ejercicio N°6](#ejercicio-n6)
   - [Ejercicio N°7](#ejercicio-n7)
3. [Repaso de Concurrencia](#parte-3-repaso-de-concurrencia)
   - [Ejercicio N°8](#ejercicio-n8)

## Parte 1: Introducción a Docker

### Ejercicio N°1

Se incorporó el archivo `generar-compose.sh` para automatizar la creación del archivo `docker-compose.yml`, permitiendo especificar un nombre y una cantidad de clientes a través de parámetros. Durante la ejecución de este archivo, se llama a una función de Python definida en `generator.py`, la cual se encarga de generar el archivo `docker-compose.yml` con el nombre y la cantidad de clientes proporcionados.

Este enfoque facilita la configuración y despliegue del entorno Docker, adaptándolo a las necesidades específicas de cada ejecución.

### Ejercicio N°2

Se modificaron tanto el cliente como el servidor para que, al realizar cambios en los archivos de configuración correspondientes, no se requiera un nuevo build de las imágenes de Docker. Para lograr esto, se actualizaron los archivos `docker-compose.yml` para crear dos volúmenes: uno para el archivo de configuración del servidor y otro para el del cliente.

Esta configuración permite que los cambios en los archivos de configuración se reflejen de inmediato sin necesidad de reconstruir las imágenes de Docker.

### Ejercicio N°3

Para validar el correcto funcionamiento del echo server, se creó el script `validar-echo-server.sh`. Este script envía el mensaje `Hello Server!` y espera recibir el mismo mensaje en respuesta.

Para realizar esta validación, se obtiene la dirección IP del servidor, que se encuentra en el contenedor de Docker. Luego, se utiliza una imagen descargada de Docker Hub (`subfuzion/netcat`), la cual incluye el comando `netcat`, evitando la necesidad de instalarlo localmente.

El script `validar-echo-server.sh` automatiza este proceso de validación y asegura que el echo server esté funcionando correctamente.

## Parte 4: Manejo de Cierre graceful

### Cierre graceful del Cliente

Para cerrar el cliente de manera graceful, se incorporó la función `handleGracefulShutdown`, que se ejecuta en un thread aparte. Se agregó un canal en el cliente que escucha señales `SIGTERM`. En caso de recibir esta señal, la función `handleGracefulShutdown` se ejecutara y se procederá a cerrar la conexión del cliente con el servidor de manera ordenada.

### Cierre graceful del Servidor

Para el cierre graceful del servidor, se utiliza la biblioteca `signal`. Cuando el servidor recibe la señal `SIGTERM`, se invoca la función `_handle_sigterm`. Esta función cierra el socket del servidor y provoca que el servidor deje de escuchar nuevas conexiones entrantes, permitiendo que los clientes existentes completen sus operaciones antes de que el servidor se cierre completamente.

## Parte 2: Repaso de Comunicaciones

### Ejercicio N°5

Nuestros clientes ahora emulan una agencia de quiniela. Reciben a través de variables de entorno nombre, apellido, DNI, nacimiento, número apostado y envían al servidor un único mensaje con la información de la apuesta.

El servidor emula una central de Lotería Nacional, la cual recibe los campos de cada apuesta en un mensaje y almacena la información mediante la función `store_bet(...)`.

#### Protocolo de mensajes

Para el correcto envío de mensajes se desarrolló un protocolo de comunicación entre el cliente y el servidor. Los primeros cuatro bytes del mensaje representan la longitud del mismo, para que tanto el servidor como el cliente sepan cuántos bytes deben leer y escribir, evitando fenómenos conocidos como `short read` y `short write`. Los campos del mensaje con la información de la apuesta se separan mediante `|`. Esta decisión se tomó porque `|` no aparece en ninguno de los archivos de apuestas de las agencias. Después de los primeros cuatro bytes del mensaje viene el ID de la agencia, que el servidor utiliza como identificador.

El mensaje final se convierte a bytes y se envía utilizando el formato big-endian. Tanto el servidor como el cliente utilizan este protocolo para asegurar la correcta escritura y lectura del mensaje a través del socket.

### Ejercicio N°6

Ahora cada cliente, en lugar de enviar un único mensaje, lee de un archivo CSV y envía varias apuestas a la vez (batches). Los batches permiten que el cliente registre varias apuestas en una misma consulta, acortando los tiempos de transmisión y procesamiento.

Para lograr esto, se incorporaron las funciones `ReadBetsFromFile`, que lee el archivo, `SendBatches`, que genera los bloques de apuestas a ser enviados al servidor, y `sendBatch`, que envía finalmente el batch al servidor a través de la función `sendMessage`, donde se abre la conexión con el servidor, se envia el mensaje y se espera por una respuesta.

#### Protocolo de mensajes

Se respeta el protocolo del lado del cliente y el servidor, donde los primeros cuatro bytes representan la longitud del mensaje y el resto el cuerpo del mensaje. Una vez enviado al servidor, este responde con un mensaje de `BATCH_RECEIVED` en caso de que la operación sea exitosa o `BATCH_FAILED` en caso de que ocurra alguna falla.

Dado que existe una limitación en el tamaño de los batches de 8kB, la función `SendBatches` se asegura de que este límite no sea excedido. Si se alcanza el límite, la parte restante del mensaje se envía en otro paquete.

### Ejercicio N°7

En este ejercicio, se incorporaron las funciones `FinishBets` y `RequestWinners` en el lado del cliente. Una vez que el cliente haya terminado de enviar las apuestas al servidor, notifica al servidor para poder solicitar los ganadores a través de la función `RequestWinners`. El sorteo solo se llevará a cabo si todos los clientes han terminado, de lo contrario, los clientes seguirán enviando mensajes al servidor solicitando los ganadores. Si todos han terminado, el servidor responderá con la lista de ganadores, si no, notificará que aún no está listo.

#### Protocolo de mensajes

Para notificar al servidor sobre la finalización del envío de apuestas, se envía un mensaje donde el header es el ID de la agencia, seguido por el delimitador `|` y la palabra `FINISHED`. Una vez que el servidor recibe este mensaje, notifica al cliente con el mensaje `FINISHED_RECEIVED`.

Después de recibir el mensaje `FINISHED_RECEIVED`, el cliente solicita los ganadores a través del mensaje `REQUEST_WINNERS`, que sigue el mismo formato que el mensaje anterior. Si no todas las agencias han terminado, el servidor responde con `NOT_READY`, y el cliente debe volver a solicitar los ganadores.

Una vez que todas las agencias han terminado, el servidor notifica a los clientes con el mensaje `WINNERS`, seguido por el DNI de los ganadores, separados por `|`.

## Parte 3: Repaso de Concurrencia

### Ejercicio N°8

En lugar de cerrar la conexión por cada mensaje, se decidió manejar una única conexión por cliente para evitar bloqueos en las conexiones entrantes. Se utilizó la biblioteca `multiprocessing` de Python para crear un nuevo proceso por cada conexión entrante. Para la sincronización, se implementaron locks para la escritura y lectura del archivo `bets.csv`, y una barrera para que cada cliente espere hasta que todos hayan terminado. Esta solución se eligió debido a que Python no soporta la ejecución paralela de hilos debido al Global Interpreter Lock (GIL).

#### Protocolo de mensajes

No se incorporaron nuevos mensajes con respecto al ejercicio anterior.
