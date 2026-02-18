# TODO - Optimización Worker Geoespacial

## Recomendaciones de optimización

1. **Reduce lock contention en el cache**
   - Considerar uso de `sync.Map` o segmentar el mapa por hash para mejorar concurrencia.

2. **Buffer de canal ajustable**
   - Ajustar el tamaño de `docsChan` según el throughput esperado.

3. **Batch insert en MongoDB**
   - Implementar inserciones en lotes para mejorar rendimiento en alta carga.

4. **Indexación en MongoDB**
   - Verificar índices en `UniqueId`, `Fecha` y/o `Location` para acelerar consultas.

5. **Configuración dinámica**
   - Permitir cambiar umbral de distancia y número de workers sin reiniciar el servicio.

6. **Monitoreo y alertas**
   - Integrar métricas con Prometheus o similar para latencia, errores, cache hit/miss y uso de memoria.

7. **Validación de datos**
   - Mejorar validaciones de coordenadas y tipos antes de procesar.

8. **Profiling y benchmarking**
   - Usar herramientas como pprof para identificar cuellos de botella en CPU, memoria y goroutines.

---

Priorizar según necesidades de negocio y carga esperada. Documentar cambios y medir impacto en producción.
