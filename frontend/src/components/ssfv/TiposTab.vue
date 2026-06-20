<script setup lang="ts">
// SSFV tab component — consumes the shared useSsfv() singleton. Behaviour and
// markup extracted verbatim from SSFVView; the parent gates mounting via v-if.
import { useSsfv } from '@/composables/useSsfv'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from '@/components/ui/table'
import { Plus, Pencil, Trash2, Cpu, Activity, Ruler } from 'lucide-vue-next'
const {
  deleteTipoEquipo,
  deleteTipoVar,
  deleteUnidad,
  openCreateTipoEquipo,
  openCreateTipoVar,
  openCreateUnidad,
  openEditTipoEquipo,
  openEditTipoVar,
  openEditUnidad,
  tipoEquipos,
  tipoVars,
  unidades,} = useSsfv()
</script>

<template>
    <div class="grid grid-cols-1 lg:grid-cols-3 gap-4">

      <!-- Tipos de Equipo -->
      <Card class="card-soft">
        <CardHeader class="px-4 py-3 border-b border-border">
          <div class="flex items-center justify-between">
            <CardTitle class="text-sm font-semibold flex items-center gap-2">
              <Cpu class="h-4 w-4 text-muted-foreground" /> Tipos de Equipo
            </CardTitle>
            <Button variant="outline" size="sm" class="h-6 text-[11px] rounded-sm" @click="openCreateTipoEquipo">
              <Plus class="h-3 w-3 mr-1" /> Nuevo
            </Button>
          </div>
        </CardHeader>
        <CardContent class="p-0">
          <Table>
            <TableHeader>
              <TableRow class="bg-muted/30">
                <TableHead class="text-[10px] uppercase tracking-[0.16em] font-bold px-4">Nombre</TableHead>
                <TableHead class="text-[10px] uppercase tracking-[0.16em] font-bold">Estado</TableHead>
                <TableHead class="w-16" />
              </TableRow>
            </TableHeader>
            <TableBody>
              <TableRow v-for="te in tipoEquipos" :key="te.tipo_id" class="border-b border-border/50 last:border-0">
                <TableCell class="px-4 text-sm font-medium">{{ te.nombre }}</TableCell>
                <TableCell>
                  <span class="text-[10px]" :class="te.activo ? 'text-green-500' : 'text-muted-foreground'">
                    {{ te.activo ? '●' : '○' }}
                  </span>
                </TableCell>
                <TableCell class="text-right pr-2">
                  <Button variant="ghost" size="icon" class="h-6 w-6" @click="openEditTipoEquipo(te)"><Pencil class="h-3 w-3" /></Button>
                  <Button variant="ghost" size="icon" class="h-6 w-6 text-destructive" @click="deleteTipoEquipo(te)"><Trash2 class="h-3 w-3" /></Button>
                </TableCell>
              </TableRow>
              <TableRow v-if="!tipoEquipos.length">
                <TableCell colspan="3" class="text-center text-muted-foreground text-xs py-4">Sin tipos de equipo</TableCell>
              </TableRow>
            </TableBody>
          </Table>
        </CardContent>
      </Card>

      <!-- Tipos de Variable -->
      <Card class="card-soft">
        <CardHeader class="px-4 py-3 border-b border-border">
          <div class="flex items-center justify-between">
            <CardTitle class="text-sm font-semibold flex items-center gap-2">
              <Activity class="h-4 w-4 text-muted-foreground" /> Tipos de Variable
            </CardTitle>
            <Button variant="outline" size="sm" class="h-6 text-[11px] rounded-sm" @click="openCreateTipoVar">
              <Plus class="h-3 w-3 mr-1" /> Nuevo
            </Button>
          </div>
        </CardHeader>
        <CardContent class="p-0">
          <Table>
            <TableHeader>
              <TableRow class="bg-muted/30">
                <TableHead class="text-[10px] uppercase tracking-[0.16em] font-bold px-4">Nombre</TableHead>
                <TableHead class="text-[10px] uppercase tracking-[0.16em] font-bold">Estado</TableHead>
                <TableHead class="w-16" />
              </TableRow>
            </TableHeader>
            <TableBody>
              <TableRow v-for="tv in tipoVars" :key="tv.tipovar_id" class="border-b border-border/50 last:border-0">
                <TableCell class="px-4 text-sm font-medium">{{ tv.nombre }}</TableCell>
                <TableCell>
                  <span class="text-[10px]" :class="tv.activo ? 'text-green-500' : 'text-muted-foreground'">{{ tv.activo ? '●' : '○' }}</span>
                </TableCell>
                <TableCell class="text-right pr-2">
                  <Button variant="ghost" size="icon" class="h-6 w-6" @click="openEditTipoVar(tv)"><Pencil class="h-3 w-3" /></Button>
                  <Button variant="ghost" size="icon" class="h-6 w-6 text-destructive" @click="deleteTipoVar(tv)"><Trash2 class="h-3 w-3" /></Button>
                </TableCell>
              </TableRow>
              <TableRow v-if="!tipoVars.length">
                <TableCell colspan="3" class="text-center text-muted-foreground text-xs py-4">Sin tipos de variable</TableCell>
              </TableRow>
            </TableBody>
          </Table>
        </CardContent>
      </Card>

      <!-- Unidades -->
      <Card class="card-soft">
        <CardHeader class="px-4 py-3 border-b border-border">
          <div class="flex items-center justify-between">
            <CardTitle class="text-sm font-semibold flex items-center gap-2">
              <Ruler class="h-4 w-4 text-muted-foreground" /> Unidades de Medida
            </CardTitle>
            <Button variant="outline" size="sm" class="h-6 text-[11px] rounded-sm" @click="openCreateUnidad">
              <Plus class="h-3 w-3 mr-1" /> Nueva
            </Button>
          </div>
        </CardHeader>
        <CardContent class="p-0">
          <Table>
            <TableHeader>
              <TableRow class="bg-muted/30">
                <TableHead class="text-[10px] uppercase tracking-[0.16em] font-bold px-4">Símbolo</TableHead>
                <TableHead class="text-[10px] uppercase tracking-[0.16em] font-bold">Magnitud</TableHead>
                <TableHead class="w-16" />
              </TableRow>
            </TableHeader>
            <TableBody>
              <TableRow v-for="u in unidades" :key="u.unidad_id" class="border-b border-border/50 last:border-0">
                <TableCell class="px-4 font-mono text-sm font-bold text-[color:var(--epm-citrico)]">{{ u.simbolo }}</TableCell>
                <TableCell class="text-xs text-muted-foreground">{{ u.magnitud }}</TableCell>
                <TableCell class="text-right pr-2">
                  <Button variant="ghost" size="icon" class="h-6 w-6" @click="openEditUnidad(u)"><Pencil class="h-3 w-3" /></Button>
                  <Button variant="ghost" size="icon" class="h-6 w-6 text-destructive" @click="deleteUnidad(u)"><Trash2 class="h-3 w-3" /></Button>
                </TableCell>
              </TableRow>
              <TableRow v-if="!unidades.length">
                <TableCell colspan="3" class="text-center text-muted-foreground text-xs py-4">Sin unidades</TableCell>
              </TableRow>
            </TableBody>
          </Table>
        </CardContent>
      </Card>
    </div>

</template>
